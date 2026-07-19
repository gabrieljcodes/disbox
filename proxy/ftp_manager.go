package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
)

type QueuedFTPJob struct {
	ID           string
	DiscordID    string
	Filename     string
	Host         string
	Username     string
	Password     string
	DownloadType string
	DownloadID   int
	FileID       int
	ClientIndex  int

	Status   string // "queued", "processing", "error", "completed"
	QueuedAt time.Time

	Progress float64
	Speed    int64
	ETA      int64
	Error    string
}

type FTPManager struct {
	server      *Server
	queue       []*QueuedFTPJob
	mu          sync.Mutex
	stopChan    chan struct{}
	globalSlots int
	activeCount int
}

func NewFTPManager(s *Server) *FTPManager {
	fm := &FTPManager{
		server:      s,
		queue:       make([]*QueuedFTPJob, 0),
		stopChan:    make(chan struct{}),
		globalSlots: 3, // global limit of 3 concurrent FTP uploads
	}
	go fm.processQueue()
	return fm
}

func (fm *FTPManager) Stop() {
	close(fm.stopChan)
}

func (fm *FTPManager) Submit(job *QueuedFTPJob) string {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	b := make([]byte, 8)
	rand.Read(b)
	job.ID = "ftp_" + hex.EncodeToString(b)
	job.Status = "queued"
	job.QueuedAt = time.Now()

	fm.queue = append(fm.queue, job)
	return job.ID
}

func (fm *FTPManager) GetQueueItems(filterID string) []QueueStatusItem {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	items := make([]QueueStatusItem, 0, len(fm.queue))
	for i, job := range fm.queue {
		if filterID != "" && job.DiscordID != filterID {
			continue
		}
		items = append(items, QueueStatusItem{
			ID:       job.ID,
			Type:     "ftp",
			Name:     "FTP: " + job.Filename,
			QueuedAt: job.QueuedAt,
			Position: i,
			Status:   job.Status,
			Progress: job.Progress,
			Speed:    job.Speed,
			ETA:      job.ETA,
		})
	}
	return items
}

func (fm *FTPManager) Remove(id string, discordID string, isAdmin bool) bool {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for i, job := range fm.queue {
		if job.ID == id {
			if !isAdmin && job.DiscordID != discordID {
				return false // Not authorized
			}
			if job.Status == "processing" {
				return false // cannot remove actively processing jobs right now
			}
			fm.queue = append(fm.queue[:i], fm.queue[i+1:]...)
			return true
		}
	}
	return false
}

func (fm *FTPManager) Move(id string, discordID string, isAdmin bool, newPosition int) bool {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if newPosition < 0 || newPosition >= len(fm.queue) {
		return false
	}

	var targetIndex = -1
	for i, job := range fm.queue {
		if job.ID == id {
			if !isAdmin && job.DiscordID != discordID {
				return false
			}
			if job.Status == "processing" {
				return false // cannot move processing job
			}
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 || targetIndex == newPosition {
		return false
	}

	job := fm.queue[targetIndex]
	fm.queue = append(fm.queue[:targetIndex], fm.queue[targetIndex+1:]...)

	fm.queue = append(fm.queue[:newPosition+1], fm.queue[newPosition:]...)
	fm.queue[newPosition] = job
	return true
}

func (fm *FTPManager) processQueue() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fm.stopChan:
			return
		case <-ticker.C:
			fm.mu.Lock()
			
			var activeJobs int
			var toRemove []int
			
			now := time.Now()
			for i, job := range fm.queue {
				if job.Status == "processing" {
					activeJobs++
				} else if job.Status == "error" || job.Status == "completed" {
					if now.Sub(job.QueuedAt) > 10 * time.Second {
						toRemove = append(toRemove, i)
					}
				}
			}
			
			for i := len(toRemove) - 1; i >= 0; i-- {
				idx := toRemove[i]
				fm.queue = append(fm.queue[:idx], fm.queue[idx+1:]...)
			}
			
			fm.activeCount = activeJobs
			
			for _, job := range fm.queue {
				if fm.activeCount >= fm.globalSlots {
					break
				}
				if job.Status == "queued" {
					job.Status = "processing"
					job.QueuedAt = time.Now() 
					fm.activeCount++
					go fm.upload(job)
				}
			}
			
			fm.mu.Unlock()
		}
	}
}

func (fm *FTPManager) upload(job *QueuedFTPJob) {
	adapter := fm.server.getAdapterForType(job.DownloadType, job.ClientIndex)
	if adapter == nil {
		fm.completeJob(job, "error", "unknown download type")
		return
	}

	downloadURL, err := adapter.RequestURL(job.DownloadID, job.FileID, "")
	if err != nil {
		fm.completeJob(job, "error", fmt.Sprintf("Failed to get URL: %v", err))
		return
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		fm.completeJob(job, "error", fmt.Sprintf("Failed to fetch file: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fm.completeJob(job, "error", fmt.Sprintf("Bad status from download: %d", resp.StatusCode))
		return
	}

	contentLength := resp.ContentLength

	host := job.Host
	if !strings.Contains(host, ":") {
		host += ":21"
	}

	c, err := ftp.Dial(host, ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		fm.completeJob(job, "error", fmt.Sprintf("FTP connect failed: %v", err))
		return
	}
	defer c.Quit()

	if err := c.Login(job.Username, job.Password); err != nil {
		fm.completeJob(job, "error", fmt.Sprintf("FTP login failed: %v", err))
		return
	}

	progressReader := &ftpProgressReader{
		r:          resp.Body,
		total:      contentLength,
		job:        job,
		fm:         fm,
		start:      time.Now(),
		lastUpdate: time.Now(),
	}

	if err := c.Stor(job.Filename, progressReader); err != nil {
		fm.completeJob(job, "error", fmt.Sprintf("FTP store failed: %v", err))
		return
	}

	fm.completeJob(job, "completed", "")
}

func (fm *FTPManager) completeJob(job *QueuedFTPJob, status string, errorMsg string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	job.Status = status
	if status == "error" {
		job.Error = errorMsg
		log.Printf("FTP Job %s failed: %s", job.ID, errorMsg)
	} else if status == "completed" {
		job.Progress = 1.0
		log.Printf("FTP Job %s completed successfully", job.ID)
	}
	job.QueuedAt = time.Now()
}

type ftpProgressReader struct {
	r          io.Reader
	total      int64
	read       int64
	job        *QueuedFTPJob
	fm         *FTPManager
	start      time.Time
	lastUpdate time.Time
}

func (pr *ftpProgressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.fm.mu.Lock()
		pr.read += int64(n)
		
		now := time.Now()
		if now.Sub(pr.lastUpdate) >= time.Second {
			if pr.total > 0 {
				pr.job.Progress = float64(pr.read) / float64(pr.total)
			}
			
			duration := now.Sub(pr.start).Seconds()
			if duration > 0 {
				pr.job.Speed = int64(float64(pr.read) / duration)
				if pr.total > 0 && pr.job.Speed > 0 {
					pr.job.ETA = (pr.total - pr.read) / pr.job.Speed
				}
			}
			pr.lastUpdate = now
		}
		pr.fm.mu.Unlock()
	}
	return n, err
}
