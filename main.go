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
	"github.com/skryvets/my-agent/internal/devcontainer"
	"github.com/skryvets/my-agent/internal/sandbox"
	"github.com/skryvets/my-agent/internal/session"
	"github.com/skryvets/my-agent/internal/task"
	"github.com/skryvets/my-agent/internal/tools"
)

func main() {
	cli := flag.Bool("cli", false, "chat in the terminal instead of serving the Telegram bot")
	// Default "taskState" is relative to the working directory: the WorkingDirectory
	// of the systemd unit on the server, or wherever the binary is started by hand.
	taskStateDir := flag.String("taskState", "taskState", "the directory the task runs are written to")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// chat answers in words, with no tools. It serves the conversations, and
	// it names the change of a task.
	chat := must(agent.New(ctx, apiKey))

	var tasks session.Tasks
	if token := os.Getenv("GITHUB_TOKEN"); token == "" {
		log.Print("/task is off: GITHUB_TOKEN is not set")
	} else {
		docker := must(sandbox.New(ctx))
		// A run removes its own container, but a run that ctx stopped may
		// not get to it, so the containers are swept here where it is certain.
		defer docker.Sweep(context.WithoutCancel(ctx))

		tasks = &task.Runner{
			Plain:   chat,
			Worker:  worker(apiKey),
			Sandbox: containers{docker},
			GitHub:  task.GitHub{Token: token},
			Store:   task.Store{Dir: *taskStateDir},
			Token:   token,
		}
	}

	serve := telegram.Run
	if *cli {
		serve = terminal.Run
	}
	if err := serve(ctx, chat, tasks); err != nil {
		log.Fatal(err)
	}
}

// worker builds the agent of one run, with every tool bound to the container
// of that run.
func worker(apiKey string) func(context.Context, tools.Workspace) (task.Agent, error) {
	return func(ctx context.Context, workspace tools.Workspace) (task.Agent, error) {
		kit, err := tools.All(workspace)
		if err != nil {
			return nil, err
		}
		return agent.New(ctx, apiKey, kit...)
	}
}

// containers lets *sandbox.Docker stand in as a task.Sandbox. Its Start
// returns a *sandbox.Container, and Go does not read that as the
// task.Container the interface asks for, so this one line does.
type containers struct{ *sandbox.Docker }

func (c containers) Start(ctx context.Context, name string, config devcontainer.Config) (task.Container, error) {
	return c.Docker.Start(ctx, name, config)
}

// must unwraps a value and an error. Every caller is start-up wiring, where
// there is nothing to fall back to.
func must[T any](value T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return value
}
