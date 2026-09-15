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
	"github.com/skryvets/my-agent/internal/task"
	"github.com/skryvets/my-agent/internal/tools"
)

func main() {
	cli := flag.Bool("cli", false, "chat in the terminal instead of serving the Telegram bot")
	stateDir := flag.String("state", "state", "the directory the task runs are written to")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	chat := agent.New(apiKey)

	if *cli {
		if err := terminal.Run(ctx, chat); err != nil {
			log.Fatal(err)
		}
		return
	}

	var options []telegram.Option
	if token := os.Getenv("GITHUB_TOKEN"); token == "" {
		log.Print("/task is off: GITHUB_TOKEN is not set")
	} else {
		pool, err := sandbox.New(ctx, sandbox.Options{})
		if err != nil {
			log.Fatal(err)
		}
		// The reaper also clears up when ctx ends, but main can return
		// first, so the containers are removed here where it is certain.
		defer pool.Shutdown(context.WithoutCancel(ctx))

		worker := agent.New(apiKey,
			tools.Shell{Workspace: pool},
			tools.ReadFile{Workspace: pool},
			tools.WriteFile{Workspace: pool},
		)
		options = append(options, telegram.WithTasks(&task.Runner{
			Agent:   worker,
			Plain:   chat,
			Sandbox: pool,
			GitHub:  task.GitHub{Token: token},
			Store:   task.Store{Dir: *stateDir},
			Token:   token,
		}))
	}

	if err := telegram.Run(ctx, chat, options...); err != nil {
		log.Fatal(err)
	}
}
