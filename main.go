package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"torbox-discord-bot/bot"
	"torbox-discord-bot/config"
	"torbox-discord-bot/proxy"
	"torbox-discord-bot/torbox"
)

func main() {
	log.Println("Starting Torbox Discord Bot...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	torboxClientPool, err := torbox.NewClientPool(cfg.TorboxAPIKeys)
	if err != nil {
		log.Fatalf("Failed to initialize Torbox client pool: %v", err)
	}

	proxyServer, err := proxy.NewServer(cfg.ProxyBaseURL, cfg.ProxyPort, torboxClientPool, cfg.DiscordClientID, cfg.DiscordClientSecret, cfg.AdminUsers)
	if err != nil {
		log.Fatalf("Failed to initialize proxy server: %v", err)
	}
	go func() {
		if err := proxyServer.Start(); err != nil {
			log.Fatalf("Failed to start proxy server: %v", err)
		}
	}()

	discordBot, err := bot.NewBot(cfg.DiscordBotToken, torboxClientPool, proxyServer, cfg.CacheOnly)
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