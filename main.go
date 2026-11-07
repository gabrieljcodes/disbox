package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"torbox-discord-bot/bot"
	"torbox-discord-bot/config"
	"torbox-discord-bot/proxy"
	"torbox-discord-bot/torbox"
	
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	isMCPMode := false
	for _, arg := range os.Args[1:] {
		if arg == "mcp" || arg == "--mcp" {
			isMCPMode = true
			break
		}
	}

	if isMCPMode {
		// In stdio MCP mode, logs must go to stderr so stdout is reserved for JSON-RPC
		log.SetOutput(os.Stderr)
		log.Println("Starting Disbox in Native MCP Server mode (stdio)...")
	} else {
		log.Println("Starting Torbox Discord Bot...")
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	torboxClientPool, err := torbox.NewClientPool([]string{})
	if err != nil {
		log.Fatalf("Failed to initialize Torbox client pool: %v", err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	proxyServer, err := proxy.NewServer(cfg, torboxClientPool, db)
	if err != nil {
		log.Fatalf("Failed to initialize proxy server: %v", err)
	}

	if isMCPMode {
		defaultUser := ""
		if len(cfg.AdminUsers) > 0 {
			defaultUser = cfg.AdminUsers[0]
		}
		if envUser := os.Getenv("DISBOX_USER_ID"); envUser != "" {
			defaultUser = envUser
		}

		if err := proxyServer.ServeMCPStdio(defaultUser); err != nil {
			log.Fatalf("MCP server error: %v", err)
		}
		return
	}

	go func() {
		if err := proxyServer.Start(); err != nil {
			log.Fatalf("Failed to start proxy server: %v", err)
		}
	}()

	discordBot, err := bot.NewBot(cfg.DiscordBotToken, torboxClientPool, proxyServer)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	if err := discordBot.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	log.Println("Bot is now running. Press CTRL+C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Shutting down...")
	discordBot.Stop()
	proxyServer.Stop()
	log.Println("Bot has been shut down gracefully.")
}