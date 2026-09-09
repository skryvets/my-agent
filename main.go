package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/communication/telegram"
	"github.com/skryvets/my-agent/internal/communication/terminal"
)

func main() {
	asBot := flag.Bool("telegram", false, "serve the agent as a Telegram bot instead of a terminal chat")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}
	model := agent.New(apiKey)

	if *asBot {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := telegram.Run(ctx, model); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := terminal.Run(context.Background(), model); err != nil {
		log.Fatal(err)
	}
}
