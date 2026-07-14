package config

import (
	"log"
	"os"
	"slices"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordBotToken      string
	DiscordClientID      string
	DiscordClientSecret  string
	AdminUsers           []string
	ProxyBaseURL         string
	ProxyPort            string
	AdminAPIEnabled      bool
	DatabaseURL          string
	EncryptionKey        string
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	proxyPort := os.Getenv("PROXY_PORT")
	if proxyPort == "" {
		proxyPort = "8080"
	}

	proxyBaseURL := os.Getenv("PROXY_BASE_URL")
	if proxyBaseURL == "" {
		proxyBaseURL = "http://localhost:" + proxyPort
	}

	cfg := &Config{
		DiscordBotToken:     os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordClientID:     os.Getenv("DISCORD_CLIENT_ID"),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		AdminUsers:          parseEnvList("ADMIN_USERS"),
		ProxyBaseURL:        proxyBaseURL,
		ProxyPort:           proxyPort,
		AdminAPIEnabled:     strings.ToLower(os.Getenv("ADMIN_API_ENABLED")) != "false",
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}

	log.Printf("Proxy server will listen on port %s", cfg.ProxyPort)
	log.Printf("Proxy base URL: %s", cfg.ProxyBaseURL)
	if cfg.DiscordClientID != "" && cfg.DiscordClientSecret != "" {
		log.Println("🌐 Web Dashboard enabled (Discord OAuth2 configured)")
	} else {
		log.Println("ℹ️  Web Dashboard disabled (set DISCORD_CLIENT_ID and DISCORD_CLIENT_SECRET to enable)")
	}

	return cfg, nil
}

func parseEnvList(envKey string) []string {
	var items []string
	envVal := os.Getenv(envKey)
	if envVal == "" {
		return items
	}

	envVal = strings.Trim(envVal, "[]")
	rawItems := strings.Split(envVal, ",")

	for _, item := range rawItems {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" && !slices.Contains(items, trimmed) {
			items = append(items, trimmed)
		}
	}
	return items
}