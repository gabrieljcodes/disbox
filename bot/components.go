package bot

import (
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) setupComponentHandlers() {
	b.Session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionMessageComponent {
			return
		}

		customID := i.MessageComponentData().CustomID

		if strings.HasPrefix(customID, "hosters_") {
			b.handleHostersButton(s, i)
		} else if strings.HasPrefix(customID, "media_search_") || strings.HasPrefix(customID, "torrent_search_") {
			b.handleSearchButtons(s, i)
		} else if strings.HasPrefix(customID, "add_torrent_") {
			b.handleAddTorrentFromSearch(s, i)
		}
	})
}

func (b *Bot) handleHostersButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Respond to the interaction immediately
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		log.Printf("Error responding to hosters button interaction: %v", err)
		return
	}

	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	if len(parts) != 3 {
		log.Printf("Invalid custom ID format: %s", customID)
		return
	}

	direction := parts[1] // "prev" or "next"
	currentPageStr := parts[2]
	currentPage, err := strconv.Atoi(currentPageStr)
	if err != nil {
		log.Printf("Failed to parse page number: %v", err)
		return
	}

	// Get hosters list
	client := b.torboxClientPool.GetCurrentClient()
	hosters, err := client.GetHosters()
	if err != nil {
		log.Printf("Error getting hosters for pagination: %v", err)
		b.sendButtonError(s, i, "Failed to load hosters list. Please try again.")
		return
	}

	if len(hosters) == 0 {
		log.Println("No hosters available")
		return
	}

	totalPages := (len(hosters) + hostersPerPage - 1) / hostersPerPage

	// Calculate new page
	newPage := currentPage
	if direction == "prev" && currentPage > 1 {
		newPage = currentPage - 1
	} else if direction == "next" && currentPage < totalPages {
		newPage = currentPage + 1
	}

	log.Printf("Hosters pagination: %s from page %d to page %d (total: %d pages)", direction, currentPage, newPage, totalPages)

	embed := createHostersEmbed(hosters, newPage, totalPages)
	components := createHostersButtons(newPage, totalPages)

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})

	if err != nil {
		log.Printf("Failed to edit hosters message: %v", err)
	}
}

func (b *Bot) sendButtonError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	embed := &discordgo.MessageEmbed{
		Title:       "❌ Error",
		Description: message,
		Color:       0xff0000,
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}