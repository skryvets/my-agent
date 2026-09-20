// Package telegram serves the agent as a Telegram bot, keeping one
// conversation per chat.
//
// Run wires the bot from the environment. github.com/go-telegram/bot polls
// getUpdates and speaks the Bot API; the per-chat conversations are in
// session.go.
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
	"sync"

	"github.com/go-telegram/bot"
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

// Bot hands each message to the session of its chat.
type Bot struct {
	api     *bot.Bot
	agent   Agent
	tasks   Tasks
	allowed map[int64]bool

	mu       sync.Mutex
	sessions map[int64]chan string
	working  map[int64]context.CancelFunc
}

// Run reads the bot configuration from the environment and serves until ctx is
// cancelled. A nil tasks answers /task with the reason it is off.
func Run(ctx context.Context, model Agent, tasks Tasks) error {
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

	b := &Bot{
		agent:    model,
		tasks:    tasks,
		allowed:  allowed,
		sessions: map[int64]chan string{},
		working:  map[int64]context.CancelFunc{},
	}
	// The handlers are not async, so the updates of a chat reach its queue in
	// the order they arrived. dispatch only queues, so nothing waits here.
	b.api, err = bot.New(token,
		bot.WithDefaultHandler(b.dispatch),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message"}),
		bot.WithNotAsyncHandlers(),
		bot.WithErrorsHandler(func(err error) { log.Printf("telegram: %v", err) }),
	)
	if err != nil {
		return err
	}

	log.Printf("telegram bot polling for updates, answering with %s", model.Model())
	b.reportInterrupted(ctx)
	b.api.Start(ctx)
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
