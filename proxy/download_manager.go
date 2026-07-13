package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"torbox-discord-bot/torbox"
)

type QueuedDownload struct {
	ID        string
	DiscordID string
	Username  string
	Avatar    string
	Type      string // "torrent", "torrent_file", "webdl"
	Link      string // for magnet or webdl
	FileData  []byte // for .torrent file
	FileName  string // for .torrent file
	CacheOnly bool
	QueuedAt  time.Time
	Status    string // "queued", "processing"

	// Results once processed
	ProxyLink   string
	ResultError error
}

type QueueStatusItem struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Name     string    `json:"name"`
	QueuedAt time.Time `json:"queued_at"`
	Position int       `json:"position"`
	Status   string    `json:"status"`
}

type CachedProgress struct {
	Progress      float64
	DownloadSpeed int64
	DownloadState string
	ETA           int64 // seconds
}

type DownloadManager struct {
	server        *Server
	queue         []*QueuedDownload
	mu            sync.Mutex
	stopChan      chan struct{}
	progressCache map[string]CachedProgress

	globalSlots int
	activeCount map[int]int    // active downloads per client index
	userActive  map[string]int // active downloads per discord user
	
	globalBandwidthLimit int64
	globalBandwidthUsed  int64
}

func NewDownloadManager(server *Server) *DownloadManager {
	dm := &DownloadManager{
		server:        server,
		queue:         make([]*QueuedDownload, 0),
		stopChan:      make(chan struct{}),
		activeCount:   make(map[int]int),
		userActive:    make(map[string]int),
		progressCache: make(map[string]CachedProgress),
	}

	go dm.processQueue()
	go dm.periodicRefresh()

	return dm
}

func (dm *DownloadManager) Stop() {
	close(dm.stopChan)
}

func (dm *DownloadManager) RefreshSlotCapacity() {
	pool := dm.server.clientPool
	totalSlots := 0

	for i := 0; i < pool.GetClientCount(); i++ {
		client := pool.GetClient(i)
		info, err := client.GetUserInfo()
		if err != nil {
			log.Printf("Warning: failed to get user info for TorBox client #%d: %v", i+1, err)
			totalSlots += 1 // fallback
			continue
		}

		slots := info.TotalSlots()
		log.Printf("TorBox client #%d has %d slots (Plan: %d, Addons: %d)", i+1, slots, info.Plan, info.AdditionalConcurrentSlots)
		totalSlots += slots
	}

	dm.mu.Lock()
	dm.globalSlots = totalSlots
	dm.mu.Unlock()
}

func (dm *DownloadManager) RefreshBandwidthUsage() {
	pool := dm.server.clientPool
	var totalLimit int64 = 0
	var totalUsed int64 = 0

	now := time.Now().UTC()
	currentMonth := now.Format("2006-01") // e.g. "2026-07"

	clientUsage := make([]int64, pool.GetClientCount())

	for i := 0; i < pool.GetClientCount(); i++ {
		client := pool.GetClient(i)
		info, err := client.GetUserInfo()
		if err != nil {
			log.Printf("Warning: failed to get user info for TorBox client #%d: %v", i+1, err)
			continue
		}

		limitBytes := torbox.PlanBandwidthLimitBytes(info.Plan)
		totalLimit += limitBytes

		stats, err := client.GetUserStats()
		if err != nil {
			log.Printf("Warning: failed to get user stats for TorBox client #%d: %v", i+1, err)
			continue
		}

		var used int64 = 0
		for _, b := range stats.Bandwidth {
			if len(b.Date) >= 7 && b.Date[:7] == currentMonth {
				used += b.BytesDownloaded
			}
		}
		
		clientUsage[i] = used
		totalUsed += used
	}

	pool.UpdateBandwidthUsage(clientUsage)

	dm.mu.Lock()
	dm.globalBandwidthLimit = totalLimit
	dm.globalBandwidthUsed = totalUsed
	dm.mu.Unlock()
}

func (dm *DownloadManager) GetSlotCapacity() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.globalSlots
}

func (dm *DownloadManager) RefreshActiveCount() {
	pool := dm.server.clientPool
	newActiveCount := make(map[int]int)
	newProgressCache := make(map[string]CachedProgress)

	for i := 0; i < pool.GetClientCount(); i++ {
		client := pool.GetClient(i)
		active := 0

		// Torrents
		torrents, errTorrents := client.ListTorrents()
		if errTorrents == nil {
			for _, t := range torrents {
				if t.Active && !t.DownloadFinished {
					active++
				}
				var eta int64 = 0
				if t.DownloadSpeed > 0 {
					eta = (t.Size - t.Downloaded) / t.DownloadSpeed
				}
				if eta < 0 {
					eta = 0
				}
				key := fmt.Sprintf("%d_torrent_%d", i, t.ID)
				newProgressCache[key] = CachedProgress{
					Progress:      t.Progress,
					DownloadSpeed: t.DownloadSpeed,
					DownloadState: t.DownloadState,
					ETA:           eta,
				}
			}
		}

		// Web downloads
		webdls, errWebDLs := client.ListWebDownloads()
		if errWebDLs == nil {
			for _, w := range webdls {
				if w.Active && !w.DownloadFinished {
					active++
				}
				var eta int64 = 0
				if w.DownloadSpeed > 0 {
					eta = (w.Size - w.Downloaded) / w.DownloadSpeed
				}
				if eta < 0 {
					eta = 0
				}
				key := fmt.Sprintf("%d_webdl_%d", i, w.ID)
				newProgressCache[key] = CachedProgress{
					Progress:      w.Progress,
					DownloadSpeed: w.DownloadSpeed,
					DownloadState: w.DownloadState,
					ETA:           eta,
				}
			}
		}

		newActiveCount[i] = active

		// Sync with DB: Mark local items as deleted if they are missing from TorBox
		if errTorrents == nil && errWebDLs == nil {
			go func(cIndex int, tList []torbox.TorrentInfo, wList []torbox.WebDownloadInfo) {
				validIDs := make(map[string]bool)
				for _, t := range tList {
					validIDs[fmt.Sprintf("torrent_%d", t.ID)] = true
				}
				for _, w := range wList {
					validIDs[fmt.Sprintf("webdl_%d", w.ID)] = true
				}

				results, err := dm.server.store.GetNonDeletedHistory(cIndex)
				if err != nil {
					return
				}

				var toDelete []string
				for _, r := range results {
					key := fmt.Sprintf("%s_%d", r.Type, r.DownloadID)
					if !validIDs[key] {
						toDelete = append(toDelete, r.Token)
					}
				}

				for _, token := range toDelete {
					dm.server.store.MarkDeleted(token)
					dm.server.store.DeleteDownloadLink(token)
					
					dm.server.mu.Lock()
					delete(dm.server.downloads, token)
					dm.server.mu.Unlock()
				}
			}(i, torrents, webdls)
		}
	}

	dm.mu.Lock()
	dm.activeCount = newActiveCount
	dm.progressCache = newProgressCache
	dm.mu.Unlock()
}

func (dm *DownloadManager) GetProgress(key string) (CachedProgress, bool) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	prog, exists := dm.progressCache[key]
	return prog, exists
}

func (dm *DownloadManager) periodicRefresh() {
	// Initial refresh
	dm.RefreshSlotCapacity()
	dm.RefreshActiveCount()
	dm.RefreshBandwidthUsage()

	// Refresh slots every hour, active count every 10 seconds
	slotTicker := time.NewTicker(1 * time.Hour)
	activeTicker := time.NewTicker(10 * time.Second)
	defer slotTicker.Stop()
	defer activeTicker.Stop()

	for {
		select {
		case <-slotTicker.C:
			dm.RefreshSlotCapacity()
			dm.RefreshBandwidthUsage()
		case <-activeTicker.C:
			dm.RefreshActiveCount()
		case <-dm.stopChan:
			return
		}
	}
}

func (dm *DownloadManager) checkUserLimit(discordID string) error {
	limitStr := dm.server.GetSetting("max_concurrent_per_user", "0")
	if limitStr == "0" || limitStr == "" {
		return nil
	}

	if dm.server.IsAdmin(discordID) {
		return nil
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return nil
	}

	dm.mu.Lock()
	userActive := dm.userActive[discordID]

	userQueued := 0
	for _, qd := range dm.queue {
		if qd.DiscordID == discordID {
			userQueued++
		}
	}
	dm.mu.Unlock()

	if userActive+userQueued >= limit {
		return fmt.Errorf("you have reached the maximum of %d concurrent downloads set by the admin", limit)
	}

	return nil
}

func (dm *DownloadManager) Submit(qd *QueuedDownload) (*QueuedDownload, error) {
	if err := dm.checkUserLimit(qd.DiscordID); err != nil {
		return nil, err
	}

	dm.mu.Lock()
	limit := dm.globalBandwidthLimit
	used := dm.globalBandwidthUsed
	totalActive := 0
	for _, count := range dm.activeCount {
		totalActive += count
	}
	availableSlots := dm.globalSlots - totalActive
	dm.mu.Unlock()

	if limit > 0 && used >= limit {
		return nil, fmt.Errorf("Global TorBox bandwidth limit exhausted for this month (%d TB used). Please try again next month.", used/(1024*1024*1024*1024))
	}

	b := make([]byte, 8)
	rand.Read(b)
	qd.ID = hex.EncodeToString(b)
	qd.QueuedAt = time.Now()

	if availableSlots > 0 {
		qd.Status = "processing"
		dm.mu.Lock()
		if len(dm.activeCount) > 0 {
			dm.activeCount[0]++
		} else {
			dm.activeCount[0] = 1
		}
		dm.userActive[qd.DiscordID]++
		dm.mu.Unlock()

		err := dm.executeDownload(qd)
		if err != nil {
			return qd, err
		}
		return qd, nil
	}

	qd.Status = "queued"
	dm.mu.Lock()
	dm.queue = append(dm.queue, qd)
	dm.mu.Unlock()
	
	// Force a dispatch attempt immediately
	go dm.tryDispatch()

	return qd, nil
}

func (dm *DownloadManager) processQueue() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dm.tryDispatch()
		case <-dm.stopChan:
			return
		}
	}
}

func (dm *DownloadManager) tryDispatch() {
	dm.mu.Lock()
	if len(dm.queue) == 0 {
		dm.mu.Unlock()
		return
	}

	totalActive := 0
	for _, count := range dm.activeCount {
		totalActive += count
	}

	availableSlots := dm.globalSlots - totalActive
	if availableSlots <= 0 {
		dm.mu.Unlock()
		return
	}

	qd := dm.queue[0]
	dm.queue = dm.queue[1:]
	qd.Status = "processing"

	if len(dm.activeCount) > 0 {
		dm.activeCount[0]++
	}
	dm.userActive[qd.DiscordID]++
	dm.mu.Unlock()

	go dm.executeDownload(qd)
}

func (dm *DownloadManager) executeDownload(qd *QueuedDownload) error {
	var err error
	var clientIndex int
	var proxyLink string
	
	if qd.Type == "torrent" {
		apiResp, cIdx, e := dm.server.clientPool.AddTorrentWithFallback(qd.Link, qd.CacheOnly)
		err = e
		clientIndex = cIdx
		if err == nil && !apiResp.Success {
			err = fmt.Errorf("%s", apiResp.Detail)
		} else if err == nil {
			data, _ := apiResp.Data.(map[string]interface{})
			id, _ := data["torrent_id"].(float64)
			name, _ := data["name"].(string)
			if name == "" {
				name = "Torrent"
			}
			proxyLink, _ = dm.server.RegisterDownloadWithUser("torrent", int(id), clientIndex, qd.DiscordID, qd.Username, qd.Avatar, name, 0)
		}
	} else if qd.Type == "torrent_file" {
		apiResp, cIdx, e := dm.server.clientPool.AddTorrentFileWithFallback(qd.FileData, qd.FileName, qd.CacheOnly)
		err = e
		clientIndex = cIdx
		if err == nil && !apiResp.Success {
			err = fmt.Errorf("%s", apiResp.Detail)
		} else if err == nil {
			data, _ := apiResp.Data.(map[string]interface{})
			id, _ := data["torrent_id"].(float64)
			name, _ := data["name"].(string)
			if name == "" {
				name = qd.FileName
			}
			proxyLink, _ = dm.server.RegisterDownloadWithUser("torrent", int(id), clientIndex, qd.DiscordID, qd.Username, qd.Avatar, name, 0)
		}
	} else if qd.Type == "webdl" {
		apiResp, cIdx, e := dm.server.clientPool.AddWebDownloadWithFallback(qd.Link)
		err = e
		clientIndex = cIdx
		if err == nil && !apiResp.Success {
			err = fmt.Errorf("%s", apiResp.Detail)
		} else if err == nil {
			data, _ := apiResp.Data.(map[string]interface{})
			id, _ := data["webdownload_id"].(float64)
			name, _ := data["name"].(string)
			if name == "" {
				name = "Web Download"
			}
			proxyLink, _ = dm.server.RegisterDownloadWithUser("webdl", int(id), clientIndex, qd.DiscordID, qd.Username, qd.Avatar, name, 0)
		}
	}

	if err != nil {
		qd.ResultError = err
		log.Printf("Download %s failed: %v", qd.ID, err)
		dm.mu.Lock()
		dm.userActive[qd.DiscordID]--
		dm.mu.Unlock()
		return err
	} else {
		log.Printf("Download %s started successfully: %s", qd.ID, proxyLink)
		qd.ProxyLink = proxyLink
		return nil
	}
}

func (dm *DownloadManager) GetQueueStatus(discordID string) []QueueStatusItem {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var items []QueueStatusItem
	for i, qd := range dm.queue {
		if qd.DiscordID == discordID || dm.server.IsAdmin(discordID) {
			name := qd.FileName
			if name == "" {
				name = qd.Link
			}
			if len(name) > 50 {
				name = name[:47] + "..."
			}
			items = append(items, QueueStatusItem{
				ID:       qd.ID,
				Type:     qd.Type,
				Name:     name,
				QueuedAt: qd.QueuedAt,
				Position: i + 1,
				Status:   qd.Status,
			})
		}
	}
	return items
}

func (dm *DownloadManager) OnDownloadComplete(discordID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.userActive[discordID] > 0 {
		dm.userActive[discordID]--
	}

	go dm.RefreshActiveCount()
}

type GlobalQueueStatus struct {
	TotalCapacity        int
	ActiveJobs           int
	QueuedJobs           int
	GlobalBandwidthLimit int64
	GlobalBandwidthUsed  int64
}

func (dm *DownloadManager) Status() GlobalQueueStatus {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	activeJobs := 0
	for _, active := range dm.activeCount {
		activeJobs += active
	}

	return GlobalQueueStatus{
		TotalCapacity:        dm.globalSlots,
		ActiveJobs:           activeJobs,
		QueuedJobs:           len(dm.queue),
		GlobalBandwidthLimit: dm.globalBandwidthLimit,
		GlobalBandwidthUsed:  dm.globalBandwidthUsed,
	}
}
