package main

import (
	"epay-bot-go/bot"
	"epay-bot-go/db"
	"epay-bot-go/service"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		token = "YOUR_BOT_TOKEN_HERE"
		log.Println("Warning: TELEGRAM_BOT_TOKEN not set, using default placeholder")
	}

	// Initialize DB
	database, err := db.NewDB("epay.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize Service
	epayService := service.NewEpayService()

	// Initialize Bot
	b, err := bot.NewBot(token, database, epayService)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Start Bot
	go b.Start()

	// Start periodic cleanup
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			log.Println("Cleaning old records...")
			if err := database.CleanOldRecords(15); err != nil {
				log.Printf("Failed to clean old records: %v", err)
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	b.Stop()
	log.Println("Goodbye!")
}
