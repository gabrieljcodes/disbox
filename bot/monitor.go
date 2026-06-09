package bot

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"torbox-discord-bot/proxy"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

type TrackedDownload struct {
	ID               int
	Type             string // "torrent" or "webdl"
	ClientIndex      int   
	UserID           string
	Username         string
	AvatarURL        string
	ChannelID        string
	MessageID        string
	Name             string
	AddedAt          time.Time
	LastMilestone    int // 0, 25, 50, 75
}

type Monitor struct {
	session         *discordgo.Session
	torboxClientPool *torbox.ClientPool
	proxyServer     *proxy.Server
	tracked         map[string]*TrackedDownload // key format: "type:id:clientIndex"
	mu              sync.RWMutex
	stopChan        chan struct{}
}

func NewMonitor(session *discordgo.Session, clientPool *torbox.ClientPool, proxyServer *proxy.Server) *Monitor {
	return &Monitor{
		session:         session,
		torboxClientPool: clientPool,
		proxyServer:     proxyServer,
		tracked:         make(map[string]*TrackedDownload),
		stopChan:        make(chan struct{}),
	}
}

func (m *Monitor) Start() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Download monitor started")

	for {
		select {
		case <-ticker.C:
			m.checkDownloads()
		case <-m.stopChan:
			log.Println("Download monitor stopped")
			return
		}
	}
}

func (m *Monitor) Stop() {
	close(m.stopChan)
}

func (m *Monitor) TrackTorrent(torrentID, clientIndex int, userID, username, avatarURL, channelID, messageID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("torrent:%d:%d", torrentID, clientIndex)
	m.tracked[key] = &TrackedDownload{
		ID:            torrentID,
		Type:          "torrent",
		ClientIndex:   clientIndex,
		UserID:        userID,
		Username:      username,
		AvatarURL:     avatarURL,
		ChannelID:     channelID,
		MessageID:     messageID,
		Name:          name,
		AddedAt:       time.Now(),
		LastMilestone: 0,
	}

	log.Printf("Now tracking torrent %d (client #%d) for user %s", torrentID, clientIndex+1, userID)
}

func (m *Monitor) TrackWebDownload(webdlID, clientIndex int, userID, username, avatarURL, channelID, messageID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("webdl:%d:%d", webdlID, clientIndex)
	m.tracked[key] = &TrackedDownload{
		ID:            webdlID,
		Type:          "webdl",
		ClientIndex:   clientIndex,
		UserID:        userID,
		Username:      username,
		AvatarURL:     avatarURL,
		ChannelID:     channelID,
		MessageID:     messageID,
		Name:          name,
		AddedAt:       time.Now(),
		LastMilestone: 0,
	}

	log.Printf("Now tracking web download %d (client #%d) for user %s", webdlID, clientIndex+1, userID)
}

func (m *Monitor) checkDownloads() {
	m.mu.RLock()
	tracked := make(map[string]*TrackedDownload)
	for k, v := range m.tracked {
		tracked[k] = v
	}
	m.mu.RUnlock()

	for key, download := range tracked {
		switch download.Type {
		case "torrent":
			m.checkTorrent(key, download)
		case "webdl":
			m.checkWebDownload(key, download)
		}
	}
}

func (m *Monitor) checkTorrent(key string, download *TrackedDownload) {
	client := m.torboxClientPool.GetClient(download.ClientIndex)
	info, err := client.GetTorrentInfo(download.ID)
	if err != nil {
		log.Printf("Error checking torrent %d (client #%d): %v", download.ID, download.ClientIndex+1, err)
		return
	}

	if info.DownloadFinished && info.DownloadPresent {
		log.Printf("Torrent %d (client #%d) has finished downloading", download.ID, download.ClientIndex+1)
		
		downloadLink, err := client.RequestDownloadURL(download.ID, -1)
		if err != nil {
			log.Printf("Failed to get download link for torrent %d (client #%d): %v", download.ID, download.ClientIndex+1, err)
			m.notifyError(download, fmt.Sprintf("Download finished but failed to get link: %v", err))
		} else {
			if info.Name != "" && download.Name != info.Name {
				download.Name = info.Name
			}
			m.notifyCompletion(download, downloadLink, info, info.Size)
		}

		m.mu.Lock()
		delete(m.tracked, key)
		m.mu.Unlock()
	} else if info.Active {
		currentMilestone := m.getMilestone(info.Progress)
		
		if currentMilestone > download.LastMilestone {
			m.notifyProgress(download, info, nil, currentMilestone)
			
			m.mu.Lock()
			download.LastMilestone = currentMilestone
			m.mu.Unlock()
		}
	}
}

func (m *Monitor) checkWebDownload(key string, download *TrackedDownload) {
	client := m.torboxClientPool.GetClient(download.ClientIndex)
	info, err := client.GetWebDownloadInfo(download.ID)
	if err != nil {
		log.Printf("Error checking web download %d (client #%d): %v", download.ID, download.ClientIndex+1, err)
		return
	}

	if info.DownloadFinished && info.DownloadPresent {
		log.Printf("Web download %d (client #%d) has finished", download.ID, download.ClientIndex+1)
		
		downloadLink, err := client.RequestWebDownloadURL(download.ID, -1)
		if err != nil {
			log.Printf("Failed to get download link for webdl %d (client #%d): %v", download.ID, download.ClientIndex+1, err)
			m.notifyError(download, fmt.Sprintf("Download finished but failed to get link: %v", err))
		} else {
			if info.Name != "" && download.Name != info.Name {
				download.Name = info.Name
			}
			m.notifyCompletion(download, downloadLink, nil, info.Size)
		}

		m.mu.Lock()
		delete(m.tracked, key)
		m.mu.Unlock()
	} else if info.Active {
		currentMilestone := m.getMilestone(info.Progress)
		
		if currentMilestone > download.LastMilestone {
			m.notifyProgress(download, nil, info, currentMilestone)
			
			m.mu.Lock()
			download.LastMilestone = currentMilestone
			m.mu.Unlock()
		}
	}
}

func (m *Monitor) getMilestone(progress float64) int {
	if progress >= 75 {
		return 75
	} else if progress >= 50 {
		return 50
	} else if progress >= 25 {
		return 25
	}
	return 0
}

func (m *Monitor) notifyCompletion(download *TrackedDownload, downloadLink string, torrentInfo *torbox.TorrentInfo, size int64) {
	log.Printf("Download %d completed using API Key #%d", download.ID, download.ClientIndex+1)
	
	// Register a proxy link instead of using the direct TorBox URL
	proxyLink, _ := m.proxyServer.RegisterDownloadWithUser(download.Type, download.ID, download.ClientIndex, download.UserID, download.Username, download.AvatarURL, download.Name, size)
	
	description := fmt.Sprintf("Your download **%s** is ready!\n\n🔒 Permanent link via proxy", download.Name)
	
	embed := &discordgo.MessageEmbed{
		Title:       "✅ Download Complete!",
		Description: description,
		Color:       0x00ff00,
		Fields:      []*discordgo.MessageEmbedField{},
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if torrentInfo != nil {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "📦 Size",
			Value:  formatBytes(torrentInfo.Size),
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "📈 Ratio",
			Value:  fmt.Sprintf("%.2f", torrentInfo.Ratio),
			Inline: true,
		})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "⚡ Status",
			Value:  "Ready to download",
			Inline: true,
		})
	}

	browseLink := strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	buttons := []discordgo.MessageComponent{
		discordgo.Button{
			Label: "🔗 Download File",
			Style: discordgo.LinkButton,
			URL:   proxyLink,
		},
		discordgo.Button{
			Label: "📂 Browse Files",
			Style: discordgo.LinkButton,
			URL:   browseLink,
		},
	}

	m.session.ChannelMessageSendComplex(download.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>", download.UserID),
		Embeds:  []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: buttons,
			},
		},
	})
}

func (m *Monitor) notifyProgress(download *TrackedDownload, torrentInfo *torbox.TorrentInfo, webdlInfo *torbox.WebDownloadInfo, milestone int) {
	var embed *discordgo.MessageEmbed

	milestoneEmoji := map[int]string{
		25: "🔵",
		50: "🟡",
		75: "🟠",
	}

	emoji := milestoneEmoji[milestone]
	if emoji == "" {
		emoji = "📊"
	}

	log.Printf("Progress update (%d%%) for download %d using API Key #%d", milestone, download.ID, download.ClientIndex+1)

	if torrentInfo != nil {
		progressBar := createProgressBar(torrentInfo.Progress)
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s Torrent Progress - %d%%", emoji, milestone),
			Description: fmt.Sprintf("**%s**", download.Name),
			Color:       0x3498db,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Progress",
					Value:  fmt.Sprintf("%s %.1f%%", progressBar, torrentInfo.Progress),
					Inline: false,
				},
				{
					Name:   "Speed",
					Value:  fmt.Sprintf("↓ %s/s | ↑ %s/s", formatBytes(torrentInfo.DownloadSpeed), formatBytes(torrentInfo.UploadSpeed)),
					Inline: true,
				},
				{
					Name:   "Seeds/Peers",
					Value:  fmt.Sprintf("🌱 %d | 👥 %d", torrentInfo.Seeds, torrentInfo.Peers),
					Inline: true,
				},
				{
					Name:   "Downloaded/Total",
					Value:  fmt.Sprintf("%s / %s", formatBytes(torrentInfo.Downloaded), formatBytes(torrentInfo.Size)),
					Inline: true,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
	} else if webdlInfo != nil {
		progressBar := createProgressBar(webdlInfo.Progress)
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s Web Download Progress - %d%%", emoji, milestone),
			Description: fmt.Sprintf("**%s**", download.Name),
			Color:       0x3498db,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Progress",
					Value:  fmt.Sprintf("%s %.1f%%", progressBar, webdlInfo.Progress),
					Inline: false,
				},
				{
					Name:   "Speed",
					Value:  fmt.Sprintf("↓ %s/s", formatBytes(webdlInfo.DownloadSpeed)),
					Inline: true,
				},
				{
					Name:   "Downloaded/Total",
					Value:  fmt.Sprintf("%s / %s", formatBytes(webdlInfo.Downloaded), formatBytes(webdlInfo.Size)),
					Inline: true,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
	}

	if embed != nil {
		m.session.ChannelMessageSendEmbed(download.ChannelID, embed)
	}
}

func (m *Monitor) notifyError(download *TrackedDownload, errorMsg string) {
	log.Printf("Error for download %d using API Key #%d: %s", download.ID, download.ClientIndex+1, errorMsg)
	
	embed := &discordgo.MessageEmbed{
		Title:       "⚠️ Download Error",
		Description: "There was a problem with your download.",
		Color:       0xff0000,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Name",
				Value: download.Name,
			},
			{
				Name:  "Error",
				Value: errorMsg,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	m.session.ChannelMessageSendComplex(download.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>", download.UserID),
		Embeds:  []*discordgo.MessageEmbed{embed},
	})
}

func createProgressBar(progress float64) string {
	blocks := int(progress / 10)
	if blocks > 10 {
		blocks = 10
	}
	
	bar := ""
	for i := 0; i < 10; i++ {
		if i < blocks {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	
	return bar
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}