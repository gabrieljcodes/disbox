package bot

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	Session      *discordgo.Session
	torboxClient *torbox.Client
	monitor      *Monitor
	commands     []*discordgo.ApplicationCommand
	handlers     map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewBot(token string, tbClient *torbox.Client) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	bot := &Bot{
		Session:      s,
		torboxClient: tbClient,
		handlers:     make(map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)),
	}

	bot.monitor = NewMonitor(s, tbClient)
	bot.defineCommands()
	bot.defineHandlers()

	return bot, nil
}

func (b *Bot) Start() error {
	b.Session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if handler, ok := b.handlers[i.ApplicationCommandData().Name]; ok {
			handler(s, i)
		}
	})

	if err := b.Session.Open(); err != nil {
		return fmt.Errorf("cannot open the session: %w", err)
	}

	log.Println("Registering commands...")
	registeredCommands, err := b.Session.ApplicationCommandBulkOverwrite(b.Session.State.User.ID, "", b.commands)
	if err != nil {
		return fmt.Errorf("cannot register commands: %w", err)
	}

	log.Println("Registered commands:")
	for _, cmd := range registeredCommands {
		log.Printf("- /%s\n", cmd.Name)
	}

	go b.monitor.Start()

	return nil
}

func (b *Bot) Stop() {
	log.Println("Deregistering commands...")
	b.monitor.Stop()
	b.Session.ApplicationCommandBulkOverwrite(b.Session.State.User.ID, "", []*discordgo.ApplicationCommand{})
	b.Session.Close()
}

func (b *Bot) defineCommands() {
	b.commands = []*discordgo.ApplicationCommand{
		{
			Name:        "add-torrent",
			Description: "Add a new torrent from a magnet link or .torrent file.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "link",
					Description: "Magnet link of the torrent",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: ".torrent file",
					Required:    false,
				},
			},
		},
		{
			Name:        "add-web-download",
			Description: "Add a new web download from a direct link.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "link",
					Description: "Direct link to the file",
					Required:    true,
				},
			},
		},
		{
			Name:        "list-downloads",
			Description: "List all active downloads (torrents and web downloads).",
		},
		{
			Name:        "torrent-status",
			Description: "Check the status of a specific torrent.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "id",
					Description: "Torrent ID",
					Required:    true,
				},
			},
		},
		{
			Name:        "webdl-status",
			Description: "Check the status of a specific web download.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "id",
					Description: "Web download ID",
					Required:    true,
				},
			},
		},
	}
}

func (b *Bot) defineHandlers() {
	b.handlers["add-torrent"] = b.handleAddTorrent
	b.handlers["add-web-download"] = b.handleAddWebDownload
	b.handlers["list-downloads"] = b.handleListDownloads
	b.handlers["torrent-status"] = b.handleTorrentStatus
	b.handlers["webdl-status"] = b.handleWebDLStatus
}

func (b *Bot) handleAddTorrent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	
	var magnetLink string
	var torrentFile []byte
	var fileName string
	var sourceDescription string
	
	for _, opt := range options {
		switch opt.Name {
		case "link":
			magnetLink = opt.StringValue()
			sourceDescription = magnetLink
		case "file":
			attachment := i.ApplicationCommandData().Resolved.Attachments[opt.Value.(string)]
			
			if !strings.HasSuffix(strings.ToLower(attachment.Filename), ".torrent") {
				b.sendError(s, i, "Error", fmt.Errorf("the file must have a .torrent extension"))
				return
			}
			
			resp, err := s.Client.Get(attachment.URL)
			if err != nil {
				b.sendError(s, i, "Error downloading file", err)
				return
			}
			defer resp.Body.Close()
			
			fileData, err := io.ReadAll(resp.Body)
			if err != nil {
				b.sendError(s, i, "Error reading file", err)
				return
			}
			
			torrentFile = fileData
			fileName = attachment.Filename
			sourceDescription = fmt.Sprintf("File: %s", fileName)
		}
	}
	
	if magnetLink == "" && len(torrentFile) == 0 {
		b.sendError(s, i, "Error", fmt.Errorf("you must provide either a magnet link OR a .torrent file"))
		return
	}
	
	var resp *torbox.APIResponse
	var err error
	
	if len(torrentFile) > 0 {
		resp, err = b.torboxClient.AddTorrentFile(torrentFile, fileName)
	} else {
		resp, err = b.torboxClient.AddTorrent(magnetLink)
	}
	
	if err != nil {
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err)
		return
	}

	if !resp.Success {
		if b.isListFullError(resp) {
			activeLimit := b.extractActiveLimit(resp)
			if activeLimit > 0 {
				err = fmt.Errorf("⚠️ Download limit reached (%d/%d active)\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.", activeLimit, activeLimit)
			} else {
				err = fmt.Errorf("⚠️ Download limit reached\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.")
			}
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err)
		} else if b.isDownloadTooLargeError(resp) {
			err = fmt.Errorf("📦 File Too Large\n\n%s\n\n💡 Try with a smaller file.", resp.Detail)
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err)
		} else {
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, resp, "", i.Member.User.ID, nil)
		}
		return
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("failed to parse API response data")
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err)
		return
	}

	torrentID, ok := data["torrent_id"].(float64)
	if !ok {
		err = fmt.Errorf("failed to parse torrent_id from API response")
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err)
		return
	}

	name, _ := data["name"].(string)
	if name == "" {
		time.Sleep(1 * time.Second)
		if info, err := b.torboxClient.GetTorrentInfo(int(torrentID)); err == nil {
			name = info.Name
		}
	}
	if name == "" {
		name = "Torrent"
	}

	// Check if torrent is already cached (instant download)
	downloadLink, err := b.torboxClient.RequestDownloadURL(int(torrentID))
	if err != nil {
		log.Printf("Torrent %d not ready yet, will monitor it: %s", int(torrentID), err)
		
		b.sendMonitoringResponse(s, i, "Torrent", name, int(torrentID), sourceDescription)
		
		msg, msgErr := s.InteractionResponse(i.Interaction)
		if msgErr == nil {
			b.monitor.TrackTorrent(int(torrentID), i.Member.User.ID, i.ChannelID, msg.ID, name)
		}
		return
	}

	b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, resp, downloadLink, i.Member.User.ID, nil)
}

func (b *Bot) handleAddWebDownload(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	downloadLink := options[0].StringValue()

	resp, err := b.torboxClient.AddWebDownload(downloadLink)
	
	if err != nil {
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err)
		return
	}

	if !resp.Success {
		if b.isListFullError(resp) {
			activeLimit := b.extractActiveLimit(resp)
			if activeLimit > 0 {
				err = fmt.Errorf("⚠️ Download limit reached (%d/%d active)\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.", activeLimit, activeLimit)
			} else {
				err = fmt.Errorf("⚠️ Download limit reached\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.")
			}
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err)
		} else if b.isDownloadTooLargeError(resp) {
			err = fmt.Errorf("📦 File Too Large\n\n%s\n\n💡 Try with a smaller file.", resp.Detail)
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err)
		} else {
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, resp, "", i.Member.User.ID, nil)
		}
		return
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("failed to parse API response data")
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err)
		return
	}

	webdlID, ok := data["webdownload_id"].(float64)
	if !ok {
		err = fmt.Errorf("failed to parse webdownload_id from API response")
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err)
		return
	}

	name := "Getting info..."
	
	time.Sleep(1 * time.Second)
	if info, err := b.torboxClient.GetWebDownloadInfo(int(webdlID)); err == nil {
		name = info.Name
	}
	
	if name == "" || name == "Getting info..." {
		name = "Web Download"
	}

	b.sendMonitoringResponse(s, i, "Web Download", name, int(webdlID), downloadLink)
	
	msg, msgErr := s.InteractionResponse(i.Interaction)
	if msgErr == nil {
		b.monitor.TrackWebDownload(int(webdlID), i.Member.User.ID, i.ChannelID, msg.ID, name)
	}
}

func (b *Bot) isListFullError(resp *torbox.APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	if resp.Error == "ACTIVE_LIMIT" {
		return true
	}
	
	if data, ok := resp.Data.(map[string]interface{}); ok {
		if errorType, exists := data["error"]; exists {
			if errorType == "ACTIVE_LIMIT" {
				return true
			}
		}
	}
	
	detailLower := strings.ToLower(resp.Detail)
	return strings.Contains(detailLower, "limit") || 
	       strings.Contains(detailLower, "active download") || 
	       strings.Contains(detailLower, "reached") ||
	       strings.Contains(detailLower, "maximum")
}

func (b *Bot) extractActiveLimit(resp *torbox.APIResponse) int {
	if resp == nil || resp.Data == nil {
		return 0
	}
	
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		return 0
	}
	
	if limitVal, exists := data["active_limit"]; exists {
		if limit, ok := limitVal.(float64); ok {
			return int(limit)
		}
	}
	
	return 0
}

func (b *Bot) isDownloadTooLargeError(resp *torbox.APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	if resp.Error == "DOWNLOAD_TOO_LARGE" {
		return true
	}
	
	detailLower := strings.ToLower(resp.Detail)
	return strings.Contains(detailLower, "too large") ||
	       strings.Contains(detailLower, "larger than")
}

func (b *Bot) handleListDownloads(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	torrents, err := b.torboxClient.ListTorrents()
	if err != nil {
		b.sendError(s, i, "Error listing torrents", err)
		return
	}

	webdls, err := b.torboxClient.ListWebDownloads()
	if err != nil {
		b.sendError(s, i, "Error listing web downloads", err)
		return
	}

	var activeTorrents []torbox.TorrentInfo
	for _, t := range torrents {
		if t.Active && !t.DownloadFinished {
			activeTorrents = append(activeTorrents, t)
		}
	}

	var activeWebDLs []torbox.WebDownloadInfo
	for _, w := range webdls {
		if w.Active && !w.DownloadFinished {
			activeWebDLs = append(activeWebDLs, w)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📥 Active Downloads",
		Description: fmt.Sprintf("Total: %d torrents, %d web downloads", len(activeTorrents), len(activeWebDLs)),
		Color:       0x3498db,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	if len(activeTorrents) > 0 {
		torrentList := ""
		for _, t := range activeTorrents {
			progressBar := createProgressBar(t.Progress)
			speed := formatBytes(t.DownloadSpeed)
			torrentList += fmt.Sprintf("**ID %d**: %s\n%s %.1f%% | 🌱 %d 👥 %d | ↓ %s/s\n\n", 
				t.ID, truncateString(t.Name, 40), progressBar, t.Progress, t.Seeds, t.Peers, speed)
		}
		
		if len(torrentList) > 1000 {
			torrentList = torrentList[:1000] + "..."
		}
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🌊 Torrents",
			Value:  torrentList,
			Inline: false,
		})
	}

	if len(activeWebDLs) > 0 {
		webdlList := ""
		for _, w := range activeWebDLs {
			progressBar := createProgressBar(w.Progress)
			speed := formatBytes(w.DownloadSpeed)
			webdlList += fmt.Sprintf("**ID %d**: %s\n%s %.1f%% | ↓ %s/s\n\n",
				w.ID, truncateString(w.Name, 40), progressBar, w.Progress, speed)
		}
		
		if len(webdlList) > 1000 {
			webdlList = webdlList[:1000] + "..."
		}
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🌐 Web Downloads",
			Value:  webdlList,
			Inline: false,
		})
	}

	if len(activeTorrents) == 0 && len(activeWebDLs) == 0 {
		embed.Description = "No active downloads at the moment."
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) handleTorrentStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	torrentID := int(options[0].IntValue())

	info, err := b.torboxClient.GetTorrentInfo(torrentID)
	if err != nil {
		b.sendError(s, i, "Error getting torrent info", err)
		return
	}

	progressBar := createProgressBar(info.Progress)
	
	stateEmoji := "⏸️"
	if info.Active {
		stateEmoji = "▶️"
	}
	if info.DownloadFinished {
		stateEmoji = "✅"
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Torrent #%d", stateEmoji, info.ID),
		Description: fmt.Sprintf("**%s**", info.Name),
		Color:       0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Progress",
				Value:  fmt.Sprintf("%s %.1f%%", progressBar, info.Progress),
				Inline: false,
			},
			{
				Name:   "Speed",
				Value:  fmt.Sprintf("↓ %s/s\n↑ %s/s", formatBytes(info.DownloadSpeed), formatBytes(info.UploadSpeed)),
				Inline: true,
			},
			{
				Name:   "Seeds/Peers",
				Value:  fmt.Sprintf("🌱 %d\n👥 %d", info.Seeds, info.Peers),
				Inline: true,
			},
			{
				Name:   "Size",
				Value:  fmt.Sprintf("%s / %s", formatBytes(info.Downloaded), formatBytes(info.Size)),
				Inline: true,
			},
			{
				Name:   "Ratio",
				Value:  fmt.Sprintf("%.2f", info.Ratio),
				Inline: true,
			},
			{
				Name:   "Status",
				Value:  info.DownloadState,
				Inline: true,
			},
		},
	}

	if info.DownloadFinished && info.DownloadPresent {
		downloadLink, err := b.torboxClient.RequestDownloadURL(torrentID)
		if err == nil {
			expirationTime := time.Now().Add(3 * time.Hour)
			expirationTimestamp := expirationTime.Unix()
			
			embed.Description = fmt.Sprintf("**%s**\n\n⏰ Link expires <t:%d:R>", info.Name, expirationTimestamp)
			
			button := discordgo.Button{
				Label: "🔗 Download File",
				Style: discordgo.LinkButton,
				URL:   downloadLink,
			}
			
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds: &[]*discordgo.MessageEmbed{embed},
				Components: &[]discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{button},
					},
				},
			})
			return
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) handleWebDLStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	webdlID := int(options[0].IntValue())

	info, err := b.torboxClient.GetWebDownloadInfo(webdlID)
	if err != nil {
		b.sendError(s, i, "Error getting web download info", err)
		return
	}

	progressBar := createProgressBar(info.Progress)
	
	stateEmoji := "⏸️"
	if info.Active {
		stateEmoji = "▶️"
	}
	if info.DownloadFinished {
		stateEmoji = "✅"
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Web Download #%d", stateEmoji, info.ID),
		Description: fmt.Sprintf("**%s**", info.Name),
		Color:       0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Progress",
				Value:  fmt.Sprintf("%s %.1f%%", progressBar, info.Progress),
				Inline: false,
			},
			{
				Name:   "Speed",
				Value:  fmt.Sprintf("↓ %s/s", formatBytes(info.DownloadSpeed)),
				Inline: true,
			},
			{
				Name:   "Size",
				Value:  fmt.Sprintf("%s / %s", formatBytes(info.Downloaded), formatBytes(info.Size)),
				Inline: true,
			},
			{
				Name:   "Status",
				Value:  info.DownloadState,
				Inline: true,
			},
		},
	}

	if info.DownloadFinished && info.DownloadPresent {
		downloadLink, err := b.torboxClient.RequestWebDownloadURL(webdlID)
		if err == nil {
			expirationTime := time.Now().Add(3 * time.Hour)
			expirationTimestamp := expirationTime.Unix()
			
			embed.Description = fmt.Sprintf("**%s**\n\n⏰ Link expires <t:%d:R>", info.Name, expirationTimestamp)
			
			button := discordgo.Button{
				Label: "🔗 Download File",
				Style: discordgo.LinkButton,
				URL:   downloadLink,
			}
			
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds: &[]*discordgo.MessageEmbed{embed},
				Components: &[]discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{button},
					},
				},
			})
			return
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) sendAPIResponseAsEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, title, link string, resp *torbox.APIResponse, downloadLink, userID string, err error) {
	var embed *discordgo.MessageEmbed
	var components []discordgo.MessageComponent
	var content string

	if err != nil {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("❌ Error to %s", title),
			Description: fmt.Sprintf("An error occurred while processing your request.\n`%s`", err.Error()),
			Color:       0xff0000,
		}
	} else if !resp.Success {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("⚠️ Failed to %s", title),
			Description: resp.Detail,
			Color:       0xffa500,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Link Sent",
					Value: fmt.Sprintf("`%s`", truncateString(link, 100)),
				},
			},
		}
	} else {
		description := resp.Detail
		if downloadLink != "" {
			expirationTime := time.Now().Add(3 * time.Hour)
			expirationTimestamp := expirationTime.Unix()
			
			content = fmt.Sprintf("<@%s>", userID)
			description = fmt.Sprintf("%s\n\n⏰ Link expires <t:%d:R>", resp.Detail, expirationTimestamp)
			
			embed = &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("✅ Success to %s", title),
				Description: description,
				Color:       0x00ff00,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:  "Link Sent",
						Value: fmt.Sprintf("`%s`", truncateString(link, 100)),
					},
				},
			}
			
			button := discordgo.Button{
				Label: "🔗 Download File",
				Style: discordgo.LinkButton,
				URL:   downloadLink,
			}
			
			components = []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{button},
				},
			}
		} else {
			embed = &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("✅ Success to %s", title),
				Description: description,
				Color:       0x00ff00,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:  "Link Sent",
						Value: fmt.Sprintf("`%s`", truncateString(link, 100)),
					},
				},
			}
		}
	}

	var contentPtr *string
	if content != "" {
		contentPtr = &content
	}
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    contentPtr,
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func (b *Bot) sendMonitoringResponse(s *discordgo.Session, i *discordgo.InteractionCreate, downloadType, name string, id int, link string) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("⏳ %s Added", downloadType),
		Description: "Your download has been added and is being monitored!",
		Color:       0xffaa00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Name",
				Value: name,
			},
			{
				Name:   "ID",
				Value:  fmt.Sprintf("%d", id),
				Inline: true,
			},
			{
				Name:   "Status",
				Value:  "Downloading...",
				Inline: true,
			},
			{
				Name:  "Link",
				Value: fmt.Sprintf("`%s`", truncateString(link, 100)),
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "You will be notified when the download finishes! Use /list-downloads to see progress.",
		},
	}

	content := fmt.Sprintf("<@%s>", i.Member.User.ID)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
		Embeds:  &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) sendError(s *discordgo.Session, i *discordgo.InteractionCreate, title string, err error) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("❌ %s", title),
		Description: fmt.Sprintf("`%s`", err.Error()),
		Color:       0xff0000,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}