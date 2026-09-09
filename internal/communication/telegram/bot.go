// Package telegram serves the agent as a Telegram bot, keeping one
// conversation per chat.
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
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

const (
	historyTurns = 10
	queueSize    = 4
)

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Model() string
	Chat(ctx context.Context, history agent.History, stream io.Writer) (agent.Message, error)
}

// Bot long-polls getUpdates and answers each message with the agent.
type Bot struct {
	client    *client
	agent     Agent
	allowed   map[int64]bool
	retryBase time.Duration

	mu       sync.Mutex
	sessions map[int64]chan string
}

// Run reads the bot configuration from the environment and serves until ctx is
// cancelled.
func Run(ctx context.Context, model Agent) error {
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
	}
	if err := bot.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

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

func (b *Bot) help() string {
	return "Send me a message and I will answer with " + b.agent.Model() + ".\n\n" +
		"/reset - forget this conversation\n" +
		"/help - show this message"
}

func (b *Bot) run(ctx context.Context) error {
	log.Printf("telegram bot polling for updates, answering with %s", b.agent.Model())

	var offset int64
	backoff := b.retryBase
	for ctx.Err() == nil {
		updates, err := b.client.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("getUpdates: %v", err)
			if !sleep(ctx, retryDelay(err, backoff)) {
				break
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = b.retryBase

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			b.dispatch(ctx, u)
		}
	}
	return ctx.Err()
}

func retryDelay(err error, backoff time.Duration) time.Duration {
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return backoff
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (b *Bot) dispatch(ctx context.Context, u update) {
	msg := u.Message
	if msg == nil || strings.TrimSpace(msg.Text) == "" {
		return
	}
	if len(b.allowed) > 0 && !b.allowed[msg.From.ID] {
		log.Printf("ignoring message from user %d (@%s)", msg.From.ID, msg.From.Username)
		return
	}

	queue := b.session(ctx, msg.Chat.ID)
	select {
	case queue <- msg.Text:
	default:
		if err := b.client.sendMessage(ctx, msg.Chat.ID, "I am still working on your previous messages, try again in a moment."); err != nil {
			log.Printf("sendMessage: %v", err)
		}
	}
}

// Each chat gets its own goroutine, so a slow answer in one chat does not block
// another. Messages inside one chat are answered in order.
func (b *Bot) session(ctx context.Context, chatID int64) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if queue, ok := b.sessions[chatID]; ok {
		return queue
	}
	queue := make(chan string, queueSize)
	b.sessions[chatID] = queue
	go b.serve(ctx, chatID, queue)
	return queue
}

func (b *Bot) serve(ctx context.Context, chatID int64, queue <-chan string) {
	var history agent.History

	for {
		var text string
		select {
		case <-ctx.Done():
			return
		case text = <-queue:
		}

		command, _, _ := strings.Cut(strings.TrimSpace(text), " ")
		switch command {
		case "/start", "/help":
			b.reply(ctx, chatID, b.help())
			continue
		case "/reset":
			history = nil
			b.reply(ctx, chatID, "Conversation cleared.")
			continue
		}

		history = history.WithUser(text)
		if err := b.client.sendChatAction(ctx, chatID, "typing"); err != nil {
			log.Printf("sendChatAction: %v", err)
		}

		assistant, err := b.agent.Chat(ctx, history, io.Discard)
		if err != nil {
			log.Printf("chat: %v", err)
			history = history.DropLast()
			b.reply(ctx, chatID, "That turn failed: "+err.Error())
			continue
		}

		history = history.WithAssistant(assistant).Trim(historyTurns * 2)
		b.reply(ctx, chatID, assistant.Content)
	}
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if err := b.client.sendMessage(ctx, chatID, text); err != nil {
		log.Printf("sendMessage: %v", err)
	}
}
