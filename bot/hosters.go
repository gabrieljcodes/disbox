package bot

import (
	"fmt"
	"strings"
	"torbox-discord-bot/torbox"

	"github.com/bwmarrin/discordgo"
)

const hostersPerPage = 5

type HosterPagination struct {
	hosters     []torbox.HosterInfo
	currentPage int
	totalPages  int
}

func (b *Bot) handleHosters(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	client := b.torboxClientPool.GetCurrentClient()
	hosters, err := client.GetHosters()
	if err != nil {
		b.sendError(s, i, "Error getting hosters", err)
		return
	}

	if len(hosters) == 0 {
		embed := &discordgo.MessageEmbed{
			Title:       "📋 Hosters",
			Description: "No hosters available at the moment.",
			Color:       0x3498db,
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
		return
	}

	totalPages := (len(hosters) + hostersPerPage - 1) / hostersPerPage
	embed := createHostersEmbed(hosters, 1, totalPages)
	components := createHostersButtons(1, totalPages)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func createHostersEmbed(hosters []torbox.HosterInfo, page, totalPages int) *discordgo.MessageEmbed {
	start := (page - 1) * hostersPerPage
	end := start + hostersPerPage
	if end > len(hosters) {
		end = len(hosters)
	}

	pageHosters := hosters[start:end]

	embed := &discordgo.MessageEmbed{
		Title:       "🌐 TorBox Hosters",
		Description: fmt.Sprintf("Available file hosters and direct download sites\n\nPage %d of %d", page, totalPages),
		Color:       0x3498db,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	for _, hoster := range pageHosters {
		statusEmoji := "❌"
		statusText := "Offline"
		if hoster.Status {
			statusEmoji = "✅"
			statusText = "Online"
		}

		// Format bandwidth limits
		dailyBandwidth := formatBytes(hoster.DailyBandwidthLimit)
		dailyBandwidthUsed := formatBytes(hoster.DailyBandwidthUsed)
		perLinkSize := formatBytes(hoster.PerLinkSizeLimit)

		// Build domains list (limit to 3 domains)
		domainsList := strings.Join(hoster.Domains[:min(3, len(hoster.Domains))], ", ")
		if len(hoster.Domains) > 3 {
			domainsList += fmt.Sprintf(" (+%d more)", len(hoster.Domains)-3)
		}

		fieldValue := fmt.Sprintf(
			"%s **Status:** %s\n"+
				"🌍 **Domains:** %s\n"+
				"🔗 **Daily Links:** %d/%d used\n"+
				"📊 **Daily Bandwidth:** %s/%s used\n"+
				"📦 **Max File Size:** %s",
			statusEmoji,
			statusText,
			domainsList,
			hoster.DailyLinkUsed,
			hoster.DailyLinkLimit,
			dailyBandwidthUsed,
			dailyBandwidth,
			perLinkSize,
		)

		if hoster.Note != nil && *hoster.Note != "" {
			fieldValue += fmt.Sprintf("\n💡 **Note:** %s", *hoster.Note)
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("**%s**", hoster.Name),
			Value:  fieldValue,
			Inline: false,
		})
	}

	// Add footer with statistics
	totalHosters := len(hosters)
	onlineCount := 0
	for _, h := range hosters {
		if h.Status {
			onlineCount++
		}
	}

	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("Total: %d hosters | Online: %d | Offline: %d", totalHosters, onlineCount, totalHosters-onlineCount),
	}

	return embed
}

func createHostersButtons(currentPage, totalPages int) []discordgo.MessageComponent {
	if totalPages <= 1 {
		return []discordgo.MessageComponent{}
	}

	leftButton := discordgo.Button{
		Label:    "⬅️",
		Style:    discordgo.PrimaryButton,
		CustomID: fmt.Sprintf("hosters_prev_%d", currentPage),
		Disabled: currentPage <= 1,
	}

	rightButton := discordgo.Button{
		Label:    "➡️",
		Style:    discordgo.PrimaryButton,
		CustomID: fmt.Sprintf("hosters_next_%d", currentPage),
		Disabled: currentPage >= totalPages,
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{leftButton, rightButton},
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (b *Bot) handleSearchHoster(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	searchQuery := strings.ToLower(strings.TrimSpace(options[0].StringValue()))

	if searchQuery == "" {
		b.sendError(s, i, "Error", fmt.Errorf("please provide a hoster name to search"))
		return
	}

	client := b.torboxClientPool.GetCurrentClient()
	hosters, err := client.GetHosters()
	if err != nil {
		b.sendError(s, i, "Error getting hosters", err)
		return
	}

	// Search for the hoster (case-insensitive, partial match)
	var foundHoster *torbox.HosterInfo
	for idx := range hosters {
		hoster := &hosters[idx]
		// Check name
		if strings.Contains(strings.ToLower(hoster.Name), searchQuery) {
			foundHoster = hoster
			break
		}
		// Check domains
		for _, domain := range hoster.Domains {
			if strings.Contains(strings.ToLower(domain), searchQuery) {
				foundHoster = hoster
				break
			}
		}
		if foundHoster != nil {
			break
		}
	}

	if foundHoster == nil {
		embed := &discordgo.MessageEmbed{
			Title:       "🔍 Hoster Not Found",
			Description: fmt.Sprintf("No hoster found matching **\"%s\"**\n\nTry using `/hosters` to see all available hosters.", searchQuery),
			Color:       0xff9800,
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
		return
	}

	// Create detailed embed for the found hoster
	embed := createDetailedHosterEmbed(foundHoster)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func createDetailedHosterEmbed(hoster *torbox.HosterInfo) *discordgo.MessageEmbed {
	statusEmoji := "❌"
	statusText := "Offline"
	statusColor := 0xff0000
	if hoster.Status {
		statusEmoji = "✅"
		statusText = "Online"
		statusColor = 0x00ff00
	}

	// Format bandwidth limits
	dailyBandwidth := formatBytes(hoster.DailyBandwidthLimit)
	dailyBandwidthUsed := formatBytes(hoster.DailyBandwidthUsed)
	bandwidthPercentage := 0.0
	if hoster.DailyBandwidthLimit > 0 {
		bandwidthPercentage = (float64(hoster.DailyBandwidthUsed) / float64(hoster.DailyBandwidthLimit)) * 100
	}

	perLinkSize := formatBytes(hoster.PerLinkSizeLimit)

	// Link usage percentage
	linkPercentage := 0.0
	if hoster.DailyLinkLimit > 0 {
		linkPercentage = (float64(hoster.DailyLinkUsed) / float64(hoster.DailyLinkLimit)) * 100
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🌐 %s", hoster.Name),
		Description: fmt.Sprintf("%s **Status:** %s", statusEmoji, statusText),
		Color:       statusColor,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: hoster.Icon,
		},
		Fields: []*discordgo.MessageEmbedField{},
	}

	// Basic Info
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "🔗 Website",
		Value:  fmt.Sprintf("[Visit Website](%s)", hoster.URL),
		Inline: false,
	})

	// Domains
	allDomains := strings.Join(hoster.Domains, "\n• ")
	if len(allDomains) > 1024 {
		allDomains = allDomains[:1021] + "..."
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   fmt.Sprintf("🌍 Supported Domains (%d)", len(hoster.Domains)),
		Value:  "• " + allDomains,
		Inline: false,
	})

	// Type
	typeEmoji := "📦"
	if hoster.Type == "hoster" {
		typeEmoji = "🗄️"
	}
	// Capitalize first letter
	typeText := hoster.Type
	if len(typeText) > 0 {
		typeText = strings.ToUpper(string(typeText[0])) + typeText[1:]
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📋 Type",
		Value:  fmt.Sprintf("%s %s", typeEmoji, typeText),
		Inline: true,
	})

	// NSFW
	nsfwEmoji := "✅"
	nsfwText := "Yes"
	if !hoster.NSFW {
		nsfwEmoji = "❌"
		nsfwText = "No"
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "🔞 NSFW",
		Value:  fmt.Sprintf("%s %s", nsfwEmoji, nsfwText),
		Inline: true,
	})

	// Daily Links
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "🔗 Daily Links",
		Value:  fmt.Sprintf("%d / %d used (%.1f%%)", hoster.DailyLinkUsed, hoster.DailyLinkLimit, linkPercentage),
		Inline: false,
	})

	// Daily Bandwidth
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📊 Daily Bandwidth",
		Value:  fmt.Sprintf("%s / %s used (%.1f%%)", dailyBandwidthUsed, dailyBandwidth, bandwidthPercentage),
		Inline: false,
	})

	// Max File Size
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📦 Max File Size per Link",
		Value:  perLinkSize,
		Inline: true,
	})

	// Note (if exists)
	if hoster.Note != nil && *hoster.Note != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "💡 Note",
			Value:  *hoster.Note,
			Inline: false,
		})
	}

	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("Hoster ID: %d", hoster.ID),
	}

	return embed
}