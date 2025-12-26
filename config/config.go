package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordBotToken string
	TorboxAPIKey    string
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		DiscordBotToken: os.Getenv("DISCORD_BOT_TOKEN"),
		TorboxAPIKey:    os.Getenv("TORBOX_API_KEY"),
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}
	if cfg.TorboxAPIKey == "" {
		log.Fatal("TORBOX_API_KEY is not set")
	}

	return cfg, nil
}