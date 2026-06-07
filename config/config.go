package config

import (
	"log"
	"os"
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
		TorboxAPIKeys:       parseTorboxAPIKeys(),
		AdminUsers:          parseAdminUsers(),
		CacheOnly:           strings.ToLower(os.Getenv("CACHE_ONLY")) == "true",
		ProxyBaseURL:        proxyBaseURL,
		ProxyPort:           proxyPort,
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

func parseAdminUsers() []string {
	var users []string

	adminEnv := os.Getenv("ADMIN_USERS")
	if adminEnv == "" {
		return users
	}

	adminEnv = strings.Trim(adminEnv, "[]")

	rawUsers := strings.Split(adminEnv, ",")

	for _, user := range rawUsers {
		trimmedUser := strings.TrimSpace(user)
		if trimmedUser != "" && !contains(users, trimmedUser) {
			users = append(users, trimmedUser)
		}
	}

	return users
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}