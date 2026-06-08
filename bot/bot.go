package bot

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"
	"torbox-discord-bot/proxy"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	Session         *discordgo.Session
	torboxClientPool *torbox.ClientPool
	proxyServer     *proxy.Server
	monitor         *Monitor
	commands        []*discordgo.ApplicationCommand
	handlers        map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewBot(token string, clientPool *torbox.ClientPool, proxyServer *proxy.Server, cacheOnly bool) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	bot := &Bot{
		Session:         s,
		torboxClientPool: clientPool,
		proxyServer:     proxyServer,
		handlers:        make(map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)),
	}

	bot.monitor = NewMonitor(s, clientPool, proxyServer)
	bot.defineCommands()
	bot.defineHandlers()

	return bot, nil
}

func (b *Bot) Start() error {
	b.Session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Only process ApplicationCommand interactions here
		if i.Type == discordgo.InteractionApplicationCommand {
			// Check access control
			if i.Member != nil && i.Member.User != nil {
				if isAllowed, reason := b.proxyServer.CheckAccess(i.Member.User.ID); !isAllowed {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "⚠️ **Access Denied**: " + reason,
							Flags:   discordgo.MessageFlagsEphemeral,
						},
					})
					return
				}
			}

			if handler, ok := b.handlers[i.ApplicationCommandData().Name]; ok {
				handler(s, i)
			}
		}
	})

	// Setup component handlers for interactive buttons
	b.setupComponentHandlers()

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
		{
			Name:        "hosters",
			Description: "List all available file hosters and their status.",
		},
		{
			Name:        "search-hoster",
			Description: "Search for detailed information about a specific hoster.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Name or domain of the hoster to search",
					Required:    true,
				},
			},
		},
		{
			Name:        "search-media",
			Description: "Search for movies, TV shows, or anime by title.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Name of the movie, TV show, or anime",
					Required:    true,
				},
			},
		},
		{
			Name:        "search-torrents",
			Description: "Search for torrents by title or name.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "What do you want to download?",
					Required:    true,
				},
			},
		},
		{
			Name:        "dashboard",
			Description: "Get a link to the web dashboard to manage your downloads.",
		},
	}
}

func (b *Bot) defineHandlers() {
	b.handlers["add-torrent"] = b.handleAddTorrent
	b.handlers["add-web-download"] = b.handleAddWebDownload
	b.handlers["list-downloads"] = b.handleListDownloads
	b.handlers["torrent-status"] = b.handleTorrentStatus
	b.handlers["webdl-status"] = b.handleWebDLStatus
	b.handlers["hosters"] = b.handleHosters
	b.handlers["search-hoster"] = b.handleSearchHoster
	b.handlers["search-media"] = b.handleSearchMedia
	b.handlers["search-torrents"] = b.handleSearchTorrents
	b.handlers["dashboard"] = b.handleDashboard
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
	var clientIndex int
	var err error
	
	if len(torrentFile) > 0 {
		resp, clientIndex, err = b.torboxClientPool.AddTorrentFileWithFallback(torrentFile, fileName, b.proxyServer.GetSetting("cache_only", "false") == "true")
	} else {
		resp, clientIndex, err = b.torboxClientPool.AddTorrentWithFallback(magnetLink, b.proxyServer.GetSetting("cache_only", "false") == "true")
	}
	
	if err != nil {
		if strings.Contains(err.Error(), "all API keys reached active limit") {
			totalKeys := b.torboxClientPool.GetClientCount()
			err = fmt.Errorf("⚠️ All API keys (%d/%d) reached download limit\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.", totalKeys, totalKeys)
		}
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err, -1)
		return
	}

	if !resp.Success {
		// Verifica se é erro de cache
		if b.isDownloadNotCachedError(resp) {
			err = fmt.Errorf("⚡ **Torrent Not Cached**\n\nThis torrent is not available in Torbox's cache.\n\n💡 To download this file, ask an admin to disable cache-only mode in the dashboard.")
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err, clientIndex)
		} else if b.isDownloadTooLargeError(resp) {
			err = fmt.Errorf("📦 File Too Large\n\n%s\n\n💡 Try with a smaller file.", resp.Detail)
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err, clientIndex)
		} else {
			b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, resp, "", i.Member.User.ID, nil, clientIndex)
		}
		return
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("failed to parse API response data")
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err, clientIndex)
		return
	}

	torrentID, ok := data["torrent_id"].(float64)
	if !ok {
		err = fmt.Errorf("failed to parse torrent_id from API response")
		b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, nil, "", i.Member.User.ID, err, clientIndex)
		return
	}

	var size int64 = 0
	name, _ := data["name"].(string)
	if name == "" {
		time.Sleep(1 * time.Second)
		client := b.torboxClientPool.GetClient(clientIndex)
		if info, err := client.GetTorrentInfo(int(torrentID)); err == nil {
			name = info.Name
			size = info.Size
		}
	}
	if name == "" {
		name = "Torrent"
	}

	client := b.torboxClientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)
	if dlErr != nil {
		log.Printf("Torrent %d not ready yet, will monitor it: %s", int(torrentID), dlErr)
		
		b.sendMonitoringResponse(s, i, "Torrent", name, int(torrentID), sourceDescription, clientIndex)
		
		msg, msgErr := s.InteractionResponse(i.Interaction)
		if msgErr == nil {
			b.monitor.TrackTorrent(int(torrentID), clientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), i.ChannelID, msg.ID, name)
		}
		return
	}

	// Register a proxy link instead of using the direct TorBox URL
	proxyLink := b.proxyServer.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), name, size)
	
	b.sendAPIResponseAsEmbed(s, i, "Add Torrent", sourceDescription, resp, proxyLink, i.Member.User.ID, nil, clientIndex)
}

func (b *Bot) handleAddWebDownload(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Check if cache-only mode is enabled
	if b.proxyServer.GetSetting("cache_only", "false") == "true" {
		err := fmt.Errorf("🚫 **Web Downloads Disabled**\n\nWeb downloads are not available in **CACHE_ONLY** mode.\n\n💡 To enable web downloads, ask an admin to disable cache-only mode in the dashboard.")
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", "", nil, "", i.Member.User.ID, err, -1)
		return
	}

	options := i.ApplicationCommandData().Options
	downloadLink := options[0].StringValue()

	resp, clientIndex, err := b.torboxClientPool.AddWebDownloadWithFallback(downloadLink)
	
	if err != nil {
		if strings.Contains(err.Error(), "all API keys reached active limit") {
			totalKeys := b.torboxClientPool.GetClientCount()
			err = fmt.Errorf("⚠️ All API keys (%d/%d) reached download limit\n\n⏳ Wait for some downloads to finish before adding new ones.\n\n💡 Use `/list-downloads` to view your active downloads.", totalKeys, totalKeys)
		}
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err, -1)
		return
	}

	if !resp.Success {
		if b.isUnsupportedHosterError(resp) {
			err = fmt.Errorf("🚫 **Unsupported File Hoster**\n\nThe website you're trying to download from is not supported by TorBox.\n\n**What you can do:**\n• Use `/hosters` to see all supported file hosting sites\n• Use `/search-hoster <name>` to check if a specific site is supported\n• Try uploading your file to a supported hoster\n\n**Need help?** Check the hosters list to find alternatives like Mega, Rapidgator, or MediaFire.")
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err, clientIndex)
		} else if b.isDownloadTooLargeError(resp) {
			err = fmt.Errorf("📦 **File Too Large**\n\n%s\n\n💡 Try with a smaller file or split it into parts.", resp.Detail)
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err, clientIndex)
		} else {
			b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, resp, "", i.Member.User.ID, nil, clientIndex)
		}
		return
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("failed to parse API response data")
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err, clientIndex)
		return
	}

	webdlID, ok := data["webdownload_id"].(float64)
	if !ok {
		err = fmt.Errorf("failed to parse webdownload_id from API response")
		b.sendAPIResponseAsEmbed(s, i, "Add Web Download", downloadLink, nil, "", i.Member.User.ID, err, clientIndex)
		return
	}

	name := "Getting info..."
	
	time.Sleep(1 * time.Second)
	client := b.torboxClientPool.GetClient(clientIndex)
	if info, err := client.GetWebDownloadInfo(int(webdlID)); err == nil {
		name = info.Name
	}
	
	if name == "" || name == "Getting info..." {
		name = "Web Download"
	}

	b.sendMonitoringResponse(s, i, "Web Download", name, int(webdlID), downloadLink, clientIndex)
	
	msg, msgErr := s.InteractionResponse(i.Interaction)
	if msgErr == nil {
		b.monitor.TrackWebDownload(int(webdlID), clientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), i.ChannelID, msg.ID, name)
	}
}

func (b *Bot) isDownloadNotCachedError(resp *torbox.APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	return resp.Error == "DOWNLOAD_NOT_CACHED"
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

func (b *Bot) isUnsupportedHosterError(resp *torbox.APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	detailLower := strings.ToLower(resp.Detail)
	return strings.Contains(detailLower, "not supported") ||
	       strings.Contains(detailLower, "unsupported")
}

func (b *Bot) handleListDownloads(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	allTorrents := []torbox.TorrentInfo{}
	allWebDLs := []torbox.WebDownloadInfo{}

	for idx := 0; idx < b.torboxClientPool.GetClientCount(); idx++ {
		client := b.torboxClientPool.GetClient(idx)
		
		torrents, err := client.ListTorrents()
		if err != nil {
			log.Printf("Error listing torrents from client #%d: %v", idx+1, err)
			continue
		}
		allTorrents = append(allTorrents, torrents...)

		webdls, err := client.ListWebDownloads()
		if err != nil {
			log.Printf("Error listing web downloads from client #%d: %v", idx+1, err)
			continue
		}
		allWebDLs = append(allWebDLs, webdls...)
	}

	var activeTorrents []torbox.TorrentInfo
	for _, t := range allTorrents {
		if t.Active && !t.DownloadFinished {
			activeTorrents = append(activeTorrents, t)
		}
	}

	var activeWebDLs []torbox.WebDownloadInfo
	for _, w := range allWebDLs {
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

	var info *torbox.TorrentInfo
	var foundClientIndex int
	var err error

	for idx := 0; idx < b.torboxClientPool.GetClientCount(); idx++ {
		client := b.torboxClientPool.GetClient(idx)
		info, err = client.GetTorrentInfo(torrentID)
		if err == nil {
			foundClientIndex = idx
			break
		}
	}

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
		// Register a proxy link (permanent, no expiration)
		proxyLink := b.proxyServer.RegisterDownloadWithUser("torrent", torrentID, foundClientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), info.Name, info.Size)
		
		embed.Description = fmt.Sprintf("**%s**\n\n🔒 Permanent link via proxy", info.Name)
		
		browseLink := strings.Replace(proxyLink, "/dl/", "/browse/", 1)
		buttons := []discordgo.MessageComponent{
			discordgo.Button{
				Label: "🔗 Download ZIP",
				Style: discordgo.LinkButton,
				URL:   proxyLink,
			},
			discordgo.Button{
				Label: "📂 Browse Files",
				Style: discordgo.LinkButton,
				URL:   browseLink,
			},
		}
		
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
			Components: &[]discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: buttons,
				},
			},
		})
		return
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

	var info *torbox.WebDownloadInfo
	var foundClientIndex int
	var err error

	for idx := 0; idx < b.torboxClientPool.GetClientCount(); idx++ {
		client := b.torboxClientPool.GetClient(idx)
		info, err = client.GetWebDownloadInfo(webdlID)
		if err == nil {
			foundClientIndex = idx
			break
		}
	}

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
		// Register a proxy link (permanent, no expiration)
		proxyLink := b.proxyServer.RegisterDownloadWithUser("webdl", webdlID, foundClientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), info.Name, info.Size)
		
		embed.Description = fmt.Sprintf("**%s**\n\n🔒 Permanent link via proxy", info.Name)
		
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
		
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
			Components: &[]discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: buttons,
				},
			},
		})
		return
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) sendAPIResponseAsEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, title, link string, resp *torbox.APIResponse, downloadLink, userID string, err error, clientIndex int) {
	var embed *discordgo.MessageEmbed
	var components []discordgo.MessageComponent
	var content string

	if clientIndex >= 0 {
		log.Printf("Response sent using API Key #%d", clientIndex+1)
	}

	if err != nil {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("❌ Error to %s", title),
			Description: fmt.Sprintf("An error occurred while processing your request.\n%s", err.Error()),
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
			content = fmt.Sprintf("<@%s>", userID)
			description = fmt.Sprintf("%s\n\n🔒 Permanent link via proxy", resp.Detail)
			
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
			
			browseLink := strings.Replace(downloadLink, "/dl/", "/browse/", 1)
			buttons := []discordgo.MessageComponent{
				discordgo.Button{
					Label: "🔗 Download File",
					Style: discordgo.LinkButton,
					URL:   downloadLink,
				},
				discordgo.Button{
					Label: "📂 Browse Files",
					Style: discordgo.LinkButton,
					URL:   browseLink,
				},
			}
			
			components = []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: buttons,
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

func (b *Bot) sendMonitoringResponse(s *discordgo.Session, i *discordgo.InteractionCreate, downloadType, name string, id int, link string, clientIndex int) {
	if clientIndex >= 0 {
		log.Printf("Monitoring %s %d using API Key #%d", downloadType, id, clientIndex+1)
	}

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

func hasMediaFiles(files []torbox.TorrentFile) bool {
	for _, f := range files {
		if isMediaName(f.Name) {
			return true
		}
	}
	return false
}

func isMediaName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".mp4") ||
		strings.HasSuffix(lower, ".mkv") ||
		strings.HasSuffix(lower, ".webm") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp")
}

func (b *Bot) handleDashboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "📦 Disbox Web Dashboard",
		Description: "Manage your downloads, view history, and add new links directly from your browser!\n\nClick the button below to access the dashboard and log in with your Discord account.",
		Color:       0x5865F2, // Discord blurple
	}

	buttons := []discordgo.MessageComponent{
		discordgo.Button{
			Label: "🌐 Open Dashboard",
			Style: discordgo.LinkButton,
			URL:   b.proxyServer.GetBaseURL() + "/dashboard",
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: buttons,
				},
			},
			Flags: discordgo.MessageFlagsEphemeral, // Only the user can see this message
		},
	})
}