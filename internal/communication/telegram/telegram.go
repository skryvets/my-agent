// Package telegram serves the agent as a Telegram bot, keeping one
// conversation per chat.
//
// Run wires the bot from the environment. The bot itself is in bot.go, the
// per-chat conversations in session.go, and the Bot API transport in client.go.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Model() string
	Chat(ctx context.Context, history agent.History, stream io.Writer) (agent.Message, error)
}

// Tasks does a coding job end to end and opens a pull request.
// *task.Runner satisfies it.
type Tasks interface {
	Start(ctx context.Context, chat, repository, instruction string, report task.Report) error
	Interrupted(ctx context.Context) []task.Run
}

// An Option changes the bot before it starts polling.
type Option func(*Bot)

// WithTasks answers /task, which clones a repository, changes it in its dev
// container and opens a pull request.
func WithTasks(tasks Tasks) Option {
	return func(b *Bot) { b.tasks = tasks }
}

// Run reads the bot configuration from the environment and serves until ctx is
// cancelled.
func Run(ctx context.Context, model Agent, options ...Option) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set")
	}
	allowed, err := parseAllowedUsers(os.Getenv("TELEGRAM_ALLOWED_USERS"))
	if err != nil {
		return err
	}
	if len(allowed) == 0 {
		log.Print("warning: TELEGRAM_ALLOWED_USERS is empty, anyone who finds the bot can spend your OpenRouter credits")
	}

	bot := &Bot{
		client:    newClient(token),
		agent:     model,
		allowed:   allowed,
		retryBase: time.Second,
		sessions:  map[int64]chan string{},
		working:   map[int64]context.CancelFunc{},
	}
	for _, option := range options {
		option(bot)
	}
	bot.reportInterrupted(ctx)
	if err := bot.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// Anyone can find a bot by its username, so without an allowlist strangers
// spend the OpenRouter credits.
func parseAllowedUsers(value string) (map[int64]bool, error) {
	allowed := map[int64]bool{}
	for _, field := range strings.Split(value, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_ALLOWED_USERS: %q is not a user id", field)
		}
		allowed[id] = true
	}
	return allowed, nil
}
