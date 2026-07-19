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
	Progress float64   `json:"progress,omitempty"`
	Speed    int64     `json:"speed,omitempty"`
	ETA      int64     `json:"eta,omitempty"`
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
	if dm.server.IsAdmin(discordID) {
		return nil
	}

	dm.mu.Lock()
	userQueued := 0
	for _, qd := range dm.queue {
		if qd.DiscordID == discordID {
			userQueued++
		}
	}
	dm.mu.Unlock()

	if userQueued >= 10 {
		return fmt.Errorf("you have reached the maximum of 10 queued downloads")
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
	globalSlots := dm.globalSlots
	totalActive := 0
	for _, count := range dm.activeCount {
		totalActive += count
	}
	availableSlots := globalSlots - totalActive
	dm.mu.Unlock()

	if globalSlots == 0 {
		return nil, fmt.Errorf("No Torbox API keys configured (or keys failed to decrypt). Please configure an API key in the dashboard.")
	}

	if limit > 0 && used >= limit {
		return nil, fmt.Errorf("Global TorBox bandwidth limit exhausted for this month (%d TB used). Please try again next month.", used/(1024*1024*1024*1024))
	}

	b := make([]byte, 8)
	rand.Read(b)
	qd.ID = hex.EncodeToString(b)
	qd.QueuedAt = time.Now()

	if availableSlots > 0 {
		limitStr := dm.server.GetSetting("max_concurrent_per_user", "0")
		limit, _ := strconv.Atoi(limitStr)
		
		dm.mu.Lock()
		userActive := dm.userActive[qd.DiscordID]
		dm.mu.Unlock()

		if limit <= 0 || dm.server.IsAdmin(qd.DiscordID) || userActive < limit {
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
			dm.mu.Lock()
			// Cleanup old error items
			now := time.Now()
			var toRemove []int
			for i, qd := range dm.queue {
				if qd.Status == "error" {
					if now.Sub(qd.QueuedAt) > 10 * time.Second {
						toRemove = append(toRemove, i)
					}
				}
			}
			for i := len(toRemove) - 1; i >= 0; i-- {
				idx := toRemove[i]
				dm.queue = append(dm.queue[:idx], dm.queue[idx+1:]...)
			}
			dm.mu.Unlock()

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

	limitStr := dm.server.GetSetting("max_concurrent_per_user", "0")
	limit, _ := strconv.Atoi(limitStr)

	var selectedQD *QueuedDownload

	for _, qd := range dm.queue {
		if qd.Status != "queued" {
			continue
		}
		if limit > 0 && !dm.server.IsAdmin(qd.DiscordID) {
			if dm.userActive[qd.DiscordID] >= limit {
				continue
			}
		}
		selectedQD = qd
		break
	}

	if selectedQD == nil {
		dm.mu.Unlock()
		return
	}

	selectedQD.Status = "processing"

	if len(dm.activeCount) > 0 {
		dm.activeCount[0]++
	} else {
		dm.activeCount[0] = 1
	}
	dm.userActive[selectedQD.DiscordID]++
	dm.mu.Unlock()

	go dm.executeDownload(selectedQD)
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
			err = torbox.FormatAPIError(apiResp)
		} else if err == nil {
			data, ok := apiResp.Data.(map[string]interface{})
			if !ok {
				err = fmt.Errorf("invalid API response data format")
			} else {
				idFloat, okID := data["torrent_id"].(float64)
				if !okID {
					err = fmt.Errorf("missing torrent_id in Torbox API response")
				} else {
					name, okName := data["name"].(string)
					if !okName || name == "" {
						name = "Torrent"
					}
					proxyLink, _ = dm.server.RegisterDownloadWithUser("torrent", int(idFloat), clientIndex, qd.DiscordID, name, 0)
				}
			}
		}
	} else if qd.Type == "torrent_file" {
		apiResp, cIdx, e := dm.server.clientPool.AddTorrentFileWithFallback(qd.FileData, qd.FileName, qd.CacheOnly)
		err = e
		clientIndex = cIdx
		if err == nil && !apiResp.Success {
			err = torbox.FormatAPIError(apiResp)
		} else if err == nil {
			data, ok := apiResp.Data.(map[string]interface{})
			if !ok {
				err = fmt.Errorf("invalid API response data format")
			} else {
				idFloat, okID := data["torrent_id"].(float64)
				if !okID {
					err = fmt.Errorf("missing torrent_id in Torbox API response")
				} else {
					name, _ := data["name"].(string)
					if name == "" {
						name = qd.FileName
					}
					proxyLink, _ = dm.server.RegisterDownloadWithUser("torrent", int(idFloat), clientIndex, qd.DiscordID, name, 0)
				}
			}
		}
	} else if qd.Type == "webdl" {
		apiResp, cIdx, e := dm.server.clientPool.AddWebDownloadWithFallback(qd.Link)
		err = e
		clientIndex = cIdx
		if err == nil && !apiResp.Success {
			err = torbox.FormatAPIError(apiResp)
		} else if err == nil {
			data, ok := apiResp.Data.(map[string]interface{})
			if !ok {
				err = fmt.Errorf("invalid API response data format")
			} else {
				idFloat, okID := data["webdownload_id"].(float64)
				if !okID {
					err = fmt.Errorf("missing webdownload_id in Torbox API response")
				} else {
					name, _ := data["name"].(string)
					if name == "" {
						name = "Web Download"
					}
					proxyLink, _ = dm.server.RegisterDownloadWithUser("webdl", int(idFloat), clientIndex, qd.DiscordID, name, 0)
				}
			}
		}
	}

	if err != nil {
		qd.ResultError = err
		log.Printf("Download %s failed: %v", qd.ID, err)
		dm.mu.Lock()
		dm.userActive[qd.DiscordID]--
		qd.Status = "error"
		qd.QueuedAt = time.Now() // reset time so cleanup logic can keep it for a bit
		dm.mu.Unlock()
		return err
	} else {
		log.Printf("Download %s started successfully: %s", qd.ID, proxyLink)
		qd.ProxyLink = proxyLink
		
		dm.mu.Lock()
		// Remove from queue upon success so it moves seamlessly to History
		for i, job := range dm.queue {
			if job.ID == qd.ID {
				dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
				break
			}
		}
		dm.mu.Unlock()
		return nil
	}
}

func (dm *DownloadManager) GetQueueItems(filterDiscordID string) []QueueStatusItem {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var items []QueueStatusItem
	for i, qd := range dm.queue {
		if filterDiscordID == "" || qd.DiscordID == filterDiscordID {
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
	if items == nil {
		items = make([]QueueStatusItem, 0)
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

func (dm *DownloadManager) RemoveFromQueue(id, discordID string, isAdmin bool) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for i, qd := range dm.queue {
		if qd.ID == id {
			if qd.DiscordID != discordID && !isAdmin {
				return fmt.Errorf("permission denied")
			}
			dm.queue = append(dm.queue[:i], dm.queue[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("item not found in queue")
}

func (dm *DownloadManager) MoveInQueue(id, discordID string, isAdmin bool, newPos int) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var qd *QueuedDownload
	oldPos := -1
	for i, item := range dm.queue {
		if item.ID == id {
			if item.DiscordID != discordID && !isAdmin {
				return fmt.Errorf("permission denied")
			}
			qd = item
			oldPos = i
			break
		}
	}

	if qd == nil {
		return fmt.Errorf("item not found in queue")
	}

	if newPos < 0 {
		newPos = 0
	}
	if newPos >= len(dm.queue) {
		newPos = len(dm.queue) - 1
	}

	if oldPos == newPos {
		return nil
	}

	dm.queue = append(dm.queue[:oldPos], dm.queue[oldPos+1:]...)

	if newPos == len(dm.queue) {
		dm.queue = append(dm.queue, qd)
	} else {
		// insert at newPos
		dm.queue = append(dm.queue[:newPos+1], dm.queue[newPos:]...)
		dm.queue[newPos] = qd
	}

	return nil
}
