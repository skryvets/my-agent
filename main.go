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
	"github.com/skryvets/my-agent/internal/sandbox"
	"github.com/skryvets/my-agent/internal/tools"
)

func main() {
	asBot := flag.Bool("telegram", false, "serve the agent as a Telegram bot instead of a terminal chat")
	boxed := flag.Bool("sandbox", false, "run the tools in a Docker container, one for each conversation")
	image := flag.String("image", sandbox.DefaultImage, "the image the sandbox containers run")
	workdir := flag.String("workdir", ".", "the host directory the tools work in, without -sandbox")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var workspace tools.Workspace = tools.Host{Dir: *workdir}
	var options []telegram.Option
	if *boxed {
		pool, err := sandbox.New(ctx, sandbox.Options{Image: *image})
		if err != nil {
			log.Fatal(err)
		}
		// The reaper also clears up when ctx ends, but main can return
		// first, so the containers are removed here where it is certain.
		defer pool.Shutdown(context.WithoutCancel(ctx))
		workspace = pool
		options = append(options, telegram.WithSandbox(pool))
	}

	model := agent.New(apiKey,
		tools.Shell{Workspace: workspace},
		tools.ReadFile{Workspace: workspace},
		tools.WriteFile{Workspace: workspace},
		tools.Fetch{},
	)

	if *asBot {
		if err := telegram.Run(ctx, model, options...); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := terminal.Run(ctx, model); err != nil {
		log.Fatal(err)
	}
}
