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
	"github.com/skryvets/my-agent/internal/tools"
)

func main() {
	asBot := flag.Bool("telegram", false, "serve the agent as a Telegram bot instead of a terminal chat")
	workdir := flag.String("workdir", ".", "the directory the tools read, write and run commands in")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}
	model := agent.New(apiKey,
		tools.Shell{Dir: *workdir},
		tools.ReadFile{Dir: *workdir},
		tools.WriteFile{Dir: *workdir},
		tools.Fetch{},
	)

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
