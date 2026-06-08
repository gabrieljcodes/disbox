package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

const (
	resultsPerPage    = 5
	torrentsPerPage   = 3
	maxDescriptionLen = 200
)

// handleSearchMedia searches for media metadata
func (b *Bot) handleSearchMedia(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	query := strings.TrimSpace(options[0].StringValue())

	if query == "" {
		b.sendError(s, i, "Error", fmt.Errorf("please provide a search query"))
		return
	}

	client := b.torboxClientPool.GetCurrentClient()
	results, err := client.SearchMetadata(query)
	if err != nil {
		b.sendError(s, i, "Error searching media", err)
		return
	}

	if len(results) == 0 {
		embed := &discordgo.MessageEmbed{
			Title:       "🔍 No Results Found",
			Description: fmt.Sprintf("No media found matching **\"%s\"**", query),
			Color:       0xff9800,
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
		return
	}

	totalPages := (len(results) + resultsPerPage - 1) / resultsPerPage
	embed := createMediaSearchEmbed(results, query, 1, totalPages)
	components := createMediaSearchButtons(query, 1, totalPages)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// handleSearchTorrents searches for torrents
func (b *Bot) handleSearchTorrents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	query := strings.TrimSpace(options[0].StringValue())

	if query == "" {
		b.sendError(s, i, "Error", fmt.Errorf("please provide a search query"))
		return
	}

	client := b.torboxClientPool.GetCurrentClient()
	searchResp, err := client.SearchTorrents(query, true) // Check cache by default
	if err != nil {
		b.sendError(s, i, "Error searching torrents", err)
		return
	}

	if len(searchResp.Torrents) == 0 {
		embed := &discordgo.MessageEmbed{
			Title:       "🔍 No Torrents Found",
			Description: fmt.Sprintf("No torrents found matching **\"%s\"**", query),
			Color:       0xff9800,
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
		return
	}

	totalPages := (len(searchResp.Torrents) + torrentsPerPage - 1) / torrentsPerPage
	embed := createTorrentSearchEmbed(searchResp, query, 1, totalPages)
	components := createTorrentSearchButtons(query, 1, totalPages, searchResp.Torrents)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func createMediaSearchEmbed(results []torbox.MetadataInfo, query string, page, totalPages int) *discordgo.MessageEmbed {
	start := (page - 1) * resultsPerPage
	end := start + resultsPerPage
	if end > len(results) {
		end = len(results)
	}

	pageResults := results[start:end]

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🎬 Search Results: \"%s\"", query),
		Description: fmt.Sprintf("Found %d results | Page %d of %d", len(results), page, totalPages),
		Color:       0x3498db,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	for idx, result := range pageResults {
		// Type emoji
		typeEmoji := "🎬"
		switch result.MediaType {
		case "movie":
			typeEmoji = "🎬"
		case "series":
			typeEmoji = "📺"
		case "anime":
			typeEmoji = "🎌"
		}

		// Format description
		description := result.Description
		if len(description) > maxDescriptionLen {
			description = description[:maxDescriptionLen] + "..."
		}

		// Format genres
		genres := "N/A"
		if len(result.Genres) > 0 {
			genres = strings.Join(result.Genres[:min(3, len(result.Genres))], ", ")
		}

		// Format rating
		rating := "N/A"
		if result.Rating > 0 {
			rating = fmt.Sprintf("⭐ %.1f/10", result.Rating)
		}

		// Format release year
		releaseYear := "N/A"
		if result.ReleaseYears != nil {
			switch v := result.ReleaseYears.(type) {
			case float64:
				releaseYear = fmt.Sprintf("%.0f", v)
			case string:
				releaseYear = v
			}
		}

		fieldValue := fmt.Sprintf(
			"%s\n\n"+
				"📅 **Year:** %s\n"+
				"🎭 **Genres:** %s\n"+
				"%s\n"+
				"🌐 **Type:** %s",
			description,
			releaseYear,
			genres,
			rating,
			strings.Title(result.MediaType),
		)

		// Add button to search torrents
		fieldName := fmt.Sprintf("%d. %s %s", start+idx+1, typeEmoji, result.Title)

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fieldName,
			Value:  fieldValue,
			Inline: false,
		})
	}

	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "💡 Use the buttons below to navigate or click 'Search Torrents' to find downloads",
	}

	return embed
}

func createTorrentSearchEmbed(searchResp *torbox.TorrentSearchResponse, query string, page, totalPages int) *discordgo.MessageEmbed {
	start := (page - 1) * torrentsPerPage
	end := start + torrentsPerPage
	if end > len(searchResp.Torrents) {
		end = len(searchResp.Torrents)
	}

	pageTorrents := searchResp.Torrents[start:end]

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🌊 Torrent Results: \"%s\"", query),
		Description: fmt.Sprintf("Found %d torrents | Page %d of %d", len(searchResp.Torrents), page, totalPages),
		Color:       0x2ecc71,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	// Add metadata info if available
	if searchResp.Metadata != nil {
		metaInfo := fmt.Sprintf(
			"**%s**\n%s\n\n",
			searchResp.Metadata.Title,
			searchResp.Metadata.Description[:min(100, len(searchResp.Metadata.Description))]+"...",
		)
		embed.Description = metaInfo + embed.Description
	}

	for idx, torrent := range pageTorrents {
		// Cache status
		cacheEmoji := "❔"
		cacheText := "Unknown"
		if torrent.Cached != nil {
			if *torrent.Cached {
				cacheEmoji = "⚡"
				cacheText = "Cached"
			} else {
				cacheEmoji = "⏳"
				cacheText = "Not Cached"
			}
		}

		// Quality info from parsed data
		quality := "Unknown"
		resolution := ""
		codec := ""
		
		if val, ok := torrent.TitleParsedData["resolution"].(string); ok && val != "" {
			resolution = val
		}
		if val, ok := torrent.TitleParsedData["quality"].(string); ok && val != "" {
			quality = val
		}
		if val, ok := torrent.TitleParsedData["codec"].(string); ok && val != "" {
			codec = val
		}

		qualityStr := quality
		if resolution != "" {
			qualityStr = fmt.Sprintf("%s %s", resolution, quality)
		}
		if codec != "" {
			qualityStr += fmt.Sprintf(" • %s", codec)
		}

		fieldValue := fmt.Sprintf(
			"%s **Cache:** %s\n"+
				"📦 **Size:** %s\n"+
				"🎬 **Quality:** %s\n"+
				"🌱 **Seeds:** %d | 👥 **Peers:** %d\n"+
				"📊 **Tracker:** %s",
			cacheEmoji,
			cacheText,
			formatBytes(torrent.Size),
			qualityStr,
			torrent.LastKnownSeeders,
			torrent.LastKnownPeers,
			torrent.Tracker,
		)

		fieldName := fmt.Sprintf("%d. %s", start+idx+1, truncateString(torrent.Title, 50))

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fieldName,
			Value:  fieldValue,
			Inline: false,
		})
	}

	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "⚡ Cached torrents download instantly • Use buttons to add torrents",
	}

	return embed
}

func createMediaSearchButtons(query string, currentPage, totalPages int) []discordgo.MessageComponent {
	buttons := []discordgo.MessageComponent{}

	// Navigation buttons
	if totalPages > 1 {
		leftButton := discordgo.Button{
			Label:    "⬅️",
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("media_search_prev_%s_%d", query, currentPage),
			Disabled: currentPage <= 1,
		}

		rightButton := discordgo.Button{
			Label:    "➡️",
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("media_search_next_%s_%d", query, currentPage),
			Disabled: currentPage >= totalPages,
		}

		buttons = append(buttons, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{leftButton, rightButton},
		})
	}

	return buttons
}

func createTorrentSearchButtons(query string, currentPage, totalPages int, allTorrents []torbox.TorrentSearchResult) []discordgo.MessageComponent {
	rows := []discordgo.MessageComponent{}

	// Navigation buttons
	if totalPages > 1 {
		leftButton := discordgo.Button{
			Label:    "⬅️",
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("torrent_search_prev_%s_%d", query, currentPage),
			Disabled: currentPage <= 1,
		}

		rightButton := discordgo.Button{
			Label:    "➡️",
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("torrent_search_next_%s_%d", query, currentPage),
			Disabled: currentPage >= totalPages,
		}

		rows = append(rows, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{leftButton, rightButton},
		})
	}

	// Add torrent buttons for current page
	start := (currentPage - 1) * torrentsPerPage
	end := start + torrentsPerPage
	if end > len(allTorrents) {
		end = len(allTorrents)
	}

	actionButtons := []discordgo.MessageComponent{}
	for idx := start; idx < end && idx < start+3; idx++ {
		torrent := allTorrents[idx]
		label := fmt.Sprintf("Add #%d", idx+1)
		
		// Emoji based on cache status
		if torrent.Cached != nil && *torrent.Cached {
			label = "⚡ " + label
		} else {
			label = "📥 " + label
		}

		button := discordgo.Button{
			Label:    label,
			Style:    discordgo.SuccessButton,
			CustomID: fmt.Sprintf("add_torrent_%s_%d", torrent.Hash, idx),
		}
		actionButtons = append(actionButtons, button)
	}

	if len(actionButtons) > 0 {
		rows = append(rows, discordgo.ActionsRow{
			Components: actionButtons,
		})
	}

	return rows
}

// Component handler for search buttons
func (b *Bot) handleSearchButtons(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		log.Printf("Error responding to search button interaction: %v", err)
		return
	}

	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")

	if len(parts) < 4 {
		log.Printf("Invalid custom ID format: %s", customID)
		return
	}

	searchType := parts[0] + "_" + parts[1] // "media_search" or "torrent_search"
	direction := parts[2]                    // "prev" or "next"
	query := parts[3]
	currentPageStr := parts[4]
	currentPage, err := strconv.Atoi(currentPageStr)
	if err != nil {
		log.Printf("Failed to parse page number: %v", err)
		return
	}

	// Calculate new page
	newPage := currentPage
	if direction == "prev" && currentPage > 1 {
		newPage = currentPage - 1
	} else if direction == "next" {
		newPage = currentPage + 1
	}

	client := b.torboxClientPool.GetCurrentClient()

	if searchType == "media_search" {
		results, err := client.SearchMetadata(query)
		if err != nil {
			log.Printf("Error re-searching media: %v", err)
			return
		}

		totalPages := (len(results) + resultsPerPage - 1) / resultsPerPage
		embed := createMediaSearchEmbed(results, query, newPage, totalPages)
		components := createMediaSearchButtons(query, newPage, totalPages)

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})

	} else if searchType == "torrent_search" {
		searchResp, err := client.SearchTorrents(query, true)
		if err != nil {
			log.Printf("Error re-searching torrents: %v", err)
			return
		}

		totalPages := (len(searchResp.Torrents) + torrentsPerPage - 1) / torrentsPerPage
		embed := createTorrentSearchEmbed(searchResp, query, newPage, totalPages)
		components := createTorrentSearchButtons(query, newPage, totalPages, searchResp.Torrents)

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})
	}
}

// Handler for add torrent button from search
func (b *Bot) handleAddTorrentFromSearch(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to add torrent button: %v", err)
		return
	}

	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	
	if len(parts) < 3 {
		log.Printf("Invalid custom ID format: %s", customID)
		return
	}

	hash := parts[2]
	
	// Get magnet link (simplified - in production you'd store this)
	magnetLink := fmt.Sprintf("magnet:?xt=urn:btih:%s", hash)
	
	resp, clientIndex, err := b.torboxClientPool.AddTorrentWithFallback(magnetLink, b.proxyServer.GetSetting("cache_only", "false") == "true")
	
	if err != nil {
		if strings.Contains(err.Error(), "all API keys reached active limit") {
			totalKeys := b.torboxClientPool.GetClientCount()
			err = fmt.Errorf("⚠️ All API keys (%d/%d) reached download limit", totalKeys, totalKeys)
		}
		b.sendSearchError(s, i, err)
		return
	}

	if !resp.Success {
		if b.isDownloadNotCachedError(resp) {
			err = fmt.Errorf("⚡ Torrent not cached. Enable full downloads in settings.")
			b.sendSearchError(s, i, err)
		} else {
			b.sendSearchError(s, i, fmt.Errorf(resp.Detail))
		}
		return
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		b.sendSearchError(s, i, fmt.Errorf("failed to parse response"))
		return
	}

	torrentID := int(data["torrent_id"].(float64))
	name, _ := data["name"].(string)

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Torrent Added!",
		Description: fmt.Sprintf("**%s** has been added to your downloads.", name),
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "ID",
				Value:  fmt.Sprintf("%d", torrentID),
				Inline: true,
			},
			{
				Name:   "API Key",
				Value:  fmt.Sprintf("#%d", clientIndex+1),
				Inline: true,
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})

	// Track the download
	client := b.torboxClientPool.GetClient(clientIndex)
	if _, err := client.GetTorrentInfo(torrentID); err == nil {
		b.monitor.TrackTorrent(torrentID, clientIndex, i.Member.User.ID, i.Member.User.Username, i.Member.User.AvatarURL(""), i.ChannelID, "", name)
	}
}

func (b *Bot) sendSearchError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	embed := &discordgo.MessageEmbed{
		Title:       "❌ Error",
		Description: err.Error(),
		Color:       0xff0000,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}