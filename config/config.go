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
	TorboxAPIKeys        []string
	AdminUsers           []string
	CacheOnly            bool
	ProxyBaseURL         string
	ProxyPort            string
	AdminAPIEnabled      bool
	DatabaseURL          string
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
		TorboxAPIKeys:       parseEnvList("TORBOX_API_KEY"),
		AdminUsers:          parseEnvList("ADMIN_USERS"),
		CacheOnly:           strings.ToLower(os.Getenv("CACHE_ONLY")) == "true",
		ProxyBaseURL:        proxyBaseURL,
		ProxyPort:           proxyPort,
		AdminAPIEnabled:     strings.ToLower(os.Getenv("ADMIN_API_ENABLED")) != "false",
		DatabaseURL:         os.Getenv("DATABASE_URL"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is not set")
	}
	if len(cfg.TorboxAPIKeys) == 0 {
		log.Fatal("No TORBOX_API_KEY found")
	}

	log.Printf("Loaded %d Torbox API key(s)", len(cfg.TorboxAPIKeys))
	log.Printf("Proxy server will listen on port %s", cfg.ProxyPort)
	log.Printf("Proxy base URL: %s", cfg.ProxyBaseURL)
	if cfg.CacheOnly {
		log.Println("⚡ CACHE_ONLY mode enabled - only cached torrents will be added")
		log.Println("🚫 Web downloads are disabled in CACHE_ONLY mode")
	}
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