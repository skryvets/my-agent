// Package telegram serves the agent as a Telegram bot, with one session for
// each chat.
//
// Run wires the bot from the environment. github.com/go-telegram/bot polls
// getUpdates and speaks the Bot API; the queue of each chat is in chat.go.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/skryvets/my-agent/internal/session"
)

// Bot hands each message to the queue of its chat.
type Bot struct {
	api     *bot.Bot
	agent   session.Agent
	tasks   session.Tasks
	allowed map[int64]bool

	mu      sync.Mutex
	queues  map[int64]chan string
	working map[int64]context.CancelFunc
}

// Run reads the bot configuration from the environment and serves until ctx is
// cancelled. A nil tasks answers /task with the reason it is off.
func Run(ctx context.Context, model session.Agent, tasks session.Tasks) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set")
	}
	allowed, err := parseAllowedUsers(os.Getenv("TELEGRAM_ALLOWED_USERS"))
	if err != nil {
		return err
	}

	b := newBot(model, tasks, allowed)
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

// newBot is the bot before it has its API client, which Run builds from the
// token and a test points at a fake.
func newBot(model session.Agent, tasks session.Tasks, allowed map[int64]bool) *Bot {
	if len(allowed) == 0 {
		log.Print("warning: TELEGRAM_ALLOWED_USERS is empty, anyone who finds the bot can spend your OpenRouter credits")
	}
	return &Bot{
		agent:   model,
		tasks:   tasks,
		allowed: allowed,
		queues:  map[int64]chan string{},
		working: map[int64]context.CancelFunc{},
	}
}

// reportInterrupted tells each chat about the run a restart caught in the
// middle.
func (b *Bot) reportInterrupted(ctx context.Context) {
	if b.tasks == nil {
		return
	}
	for _, run := range b.tasks.Interrupted(ctx) {
		chatID, err := strconv.ParseInt(run.Chat, 10, 64)
		if err != nil {
			continue
		}
		b.reply(ctx, chatID, session.Lost(run))
	}
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
