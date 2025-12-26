package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordBotToken string
	TorboxAPIKeys   []string
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		DiscordBotToken: os.Getenv("DISCORD_BOT_TOKEN"),
		TorboxAPIKeys:   parseTorboxAPIKeys(),
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}
	if len(cfg.TorboxAPIKeys) == 0 {
		log.Fatal("No TORBOX_API_KEY found")
	}

	log.Printf("Loaded %d Torbox API key(s)", len(cfg.TorboxAPIKeys))

	return cfg, nil
}

func parseTorboxAPIKeys() []string {
	var keys []string

	apiKeyEnv := os.Getenv("TORBOX_API_KEY")
	if apiKeyEnv == "" {
		return keys
	}

	apiKeyEnv = strings.Trim(apiKeyEnv, "[]")

	rawKeys := strings.Split(apiKeyEnv, ",")

	for _, key := range rawKeys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey != "" && !contains(keys, trimmedKey) {
			keys = append(keys, trimmedKey)
		}
	}

	return keys
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}