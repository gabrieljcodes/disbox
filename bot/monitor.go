package bot

import (
	"fmt"
	"log"
	"sync"
	"time"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

type TrackedDownload struct {
	ID               int
	Type             string // "torrent" or "webdl"
	UserID           string
	ChannelID        string
	MessageID        string
	Name             string
	AddedAt          time.Time
	LastMilestone    int // 0, 25, 50, 75
}

type Monitor struct {
	session      *discordgo.Session
	torboxClient *torbox.Client
	tracked      map[string]*TrackedDownload // key format: "type:id"
	mu           sync.RWMutex
	stopChan     chan struct{}
}

func NewMonitor(session *discordgo.Session, torboxClient *torbox.Client) *Monitor {
	return &Monitor{
		session:      session,
		torboxClient: torboxClient,
		tracked:      make(map[string]*TrackedDownload),
		stopChan:     make(chan struct{}),
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

func (m *Monitor) TrackTorrent(torrentID int, userID, channelID, messageID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("torrent:%d", torrentID)
	m.tracked[key] = &TrackedDownload{
		ID:            torrentID,
		Type:          "torrent",
		UserID:        userID,
		ChannelID:     channelID,
		MessageID:     messageID,
		Name:          name,
		AddedAt:       time.Now(),
		LastMilestone: 0,
	}

	log.Printf("Now tracking torrent %d for user %s", torrentID, userID)
}

func (m *Monitor) TrackWebDownload(webdlID int, userID, channelID, messageID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("webdl:%d", webdlID)
	m.tracked[key] = &TrackedDownload{
		ID:            webdlID,
		Type:          "webdl",
		UserID:        userID,
		ChannelID:     channelID,
		MessageID:     messageID,
		Name:          name,
		AddedAt:       time.Now(),
		LastMilestone: 0,
	}

	log.Printf("Now tracking web download %d for user %s", webdlID, userID)
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
	info, err := m.torboxClient.GetTorrentInfo(download.ID)
	if err != nil {
		log.Printf("Error checking torrent %d: %v", download.ID, err)
		return
	}

	if info.DownloadFinished && info.DownloadPresent {
		log.Printf("Torrent %d has finished downloading", download.ID)
		
		downloadLink, err := m.torboxClient.RequestDownloadURL(download.ID)
		if err != nil {
			log.Printf("Failed to get download link for torrent %d: %v", download.ID, err)
			m.notifyError(download, fmt.Sprintf("Download finished but failed to get link: %v", err))
		} else {
			m.notifyCompletion(download, downloadLink, info)
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
	info, err := m.torboxClient.GetWebDownloadInfo(download.ID)
	if err != nil {
		log.Printf("Error checking web download %d: %v", download.ID, err)
		return
	}

	if info.DownloadFinished && info.DownloadPresent {
		log.Printf("Web download %d has finished", download.ID)
		
		downloadLink, err := m.torboxClient.RequestWebDownloadURL(download.ID)
		if err != nil {
			log.Printf("Failed to get download link for webdl %d: %v", download.ID, err)
			m.notifyError(download, fmt.Sprintf("Download finished but failed to get link: %v", err))
		} else {
			m.notifyCompletion(download, downloadLink, nil)
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

func (m *Monitor) notifyCompletion(download *TrackedDownload, downloadLink string, torrentInfo *torbox.TorrentInfo) {
	expirationTime := time.Now().Add(3 * time.Hour)
	expirationTimestamp := expirationTime.Unix()
	
	description := fmt.Sprintf("Your download **%s** is ready!\n\n⏰ Link expires <t:%d:R>", download.Name, expirationTimestamp)
	
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

	button := discordgo.Button{
		Label: "🔗 Download File",
		Style: discordgo.LinkButton,
		URL:   downloadLink,
	}

	m.session.ChannelMessageSendComplex(download.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>", download.UserID),
		Embeds:  []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{button},
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