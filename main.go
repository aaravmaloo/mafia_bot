package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mafia-bot/bot"
	"mafia-bot/commands"
	"mafia-bot/config"
	"mafia-bot/game"
	sessionstore "mafia-bot/store"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	groupJID, err := cfg.ParsedGroupJID()
	if err != nil {
		log.Fatalf("parse group JID: %v", err)
	}

	ctx := context.Background()
	container, deviceStore, err := sessionstore.OpenDeviceStore(ctx, cfg.DatabaseDriver, cfg.DatabaseDSN, cfg.LogLevel)
	if err != nil {
		log.Fatalf("open whatsmeow store: %v", err)
	}
	defer func() {
		_ = container.Close()
	}()

	client := bot.NewClient(deviceStore, cfg.LogLevel)
	sender := bot.NewSender(client.WA)
	games := game.NewRegistry()
	router := commands.NewRouter(cfg, groupJID, games, sender)
	handler := bot.NewHandler(client, sender, router, games, groupJID, cfg.Prefix)

	client.WA.AddEventHandler(handler.HandleEvent)

	if err = client.Connect(ctx); err != nil {
		log.Fatalf("connect bot: %v", err)
	}

	waitForShutdown()
	client.Disconnect()
}

func waitForShutdown() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}
