package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/approval"
	"github.com/skryvets/my-agent/internal/communication/telegram"
	"github.com/skryvets/my-agent/internal/communication/terminal"
	"github.com/skryvets/my-agent/internal/sandbox"
	"github.com/skryvets/my-agent/internal/task"
	"github.com/skryvets/my-agent/internal/tools"
)

func main() {
	asBot := flag.Bool("telegram", false, "serve the agent as a Telegram bot instead of a terminal chat")
	boxed := flag.Bool("sandbox", false, "run the tools in a Docker container, one for each conversation")
	image := flag.String("image", sandbox.DefaultImage, "the image the sandbox containers run")
	workdir := flag.String("workdir", ".", "the host directory the tools work in, without -sandbox")
	gated := flag.Bool("approval", true, "ask before a tool call the policy does not allow by itself")
	stateDir := flag.String("state", "state", "the directory the task runs are written to")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var workspace tools.Workspace = tools.Host{Dir: *workdir}
	var options []telegram.Option
	var pool *sandbox.Pool
	if *boxed {
		started, err := sandbox.New(ctx, sandbox.Options{Image: *image})
		if err != nil {
			log.Fatal(err)
		}
		pool = started
		// The reaper also clears up when ctx ends, but main can return
		// first, so the containers are removed here where it is certain.
		defer pool.Shutdown(context.WithoutCancel(ctx))
		workspace = pool
		options = append(options, telegram.WithSandbox(pool))
	}

	kit := []agent.Tool{
		tools.Shell{Workspace: workspace},
		tools.ReadFile{Workspace: workspace},
		tools.WriteFile{Workspace: workspace},
		tools.Fetch{},
	}

	broker := &approval.Broker{}
	if *gated {
		kit = approval.Guarded(broker, approval.Default(), kit...)
	}
	model := agent.New(apiKey, kit...)

	if *asBot {
		if *gated {
			options = append(options, telegram.WithApproval(broker))
		}
		// A task needs a container to work in and a token to push with.
		token := os.Getenv("GITHUB_TOKEN")
		switch {
		case pool == nil:
			log.Print("/task is off: start me with -sandbox")
		case token == "":
			log.Print("/task is off: GITHUB_TOKEN is not set")
		default:
			options = append(options, telegram.WithTasks(&task.Runner{
				Agent: model,
				// Naming the change needs no tool, and a question
				// that carries none is answered sooner.
				Plain:   agent.New(apiKey),
				Sandbox: pool,
				GitHub:  task.GitHub{Token: token},
				Store:   task.Store{Dir: *stateDir},
				Token:   token,
			}))
		}
		if err := telegram.Run(ctx, model, options...); err != nil {
			log.Fatal(err)
		}
		return
	}

	var chat []terminal.Option
	if *gated {
		chat = append(chat, terminal.WithApproval(broker))
	}
	if err := terminal.Run(ctx, model, chat...); err != nil {
		log.Fatal(err)
	}
}
