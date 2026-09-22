package telegram

import (
	"context"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/skryvets/my-agent/internal/agent"
)

const (
	// historyTurns caps how much of a conversation is replayed to the model.
	// History lives in memory only, so a restart clears it.
	historyTurns = 10

	// queueSize is how far a sender may run ahead of the answers.
	queueSize = 4
)

// dispatch drops what the bot will not answer and queues the rest.
func (b *Bot) dispatch(ctx context.Context, _ *bot.Bot, u *models.Update) {
	msg := u.Message
	if msg == nil || msg.From == nil || strings.TrimSpace(msg.Text) == "" {
		return
	}
	if len(b.allowed) > 0 && !b.allowed[msg.From.ID] {
		log.Printf("ignoring message from user %d (@%s)", msg.From.ID, msg.From.Username)
		return
	}
	if commandName(msg.Text) == "/stop" {
		b.stop(ctx, msg.Chat.ID)
		return
	}

	queue := b.session(ctx, msg.Chat.ID)
	select {
	case queue <- msg.Text:
	default:
		b.reply(ctx, msg.Chat.ID, "I am still working on your previous messages, try again in a moment. /stop drops them.")
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

// serve owns the history for one chat, so no lock is needed around it.
func (b *Bot) serve(ctx context.Context, chatID int64, queue <-chan string) {
	var history agent.History
	for {
		select {
		case <-ctx.Done():
			return
		case text := <-queue:
			history = b.handle(ctx, chatID, history, text)
		}
	}
}

// handle answers one message and returns the history after it. ctx lives as
// long as the bot and carries the replies. work lives as long as this message
// and ends early on /stop.
func (b *Bot) handle(ctx context.Context, chatID int64, history agent.History, text string) agent.History {
	work, done := b.begin(ctx, chatID)
	defer done()

	if handled, cleared := b.command(ctx, work, chatID, text); handled {
		if cleared {
			return nil
		}
		return history
	}

	history = history.WithUser(text)
	if _, err := b.api.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	}); err != nil {
		log.Printf("sendChatAction: %v", err)
	}
	answer, err := b.agent.Chat(work, history, io.Discard)
	if err != nil {
		if work.Err() == nil {
			log.Printf("chat: %v", err)
			b.reply(ctx, chatID, "That turn failed: "+err.Error())
		}
		return history.DropLast()
	}
	b.reply(ctx, chatID, answer)
	return history.WithAssistant(answer).Trim(historyTurns * 2)
}

// command answers a slash command, reporting whether it handled the message and
// whether the conversation should be cleared.
func (b *Bot) command(ctx, work context.Context, chatID int64, text string) (handled, cleared bool) {
	switch commandName(text) {
	case "/start", "/help":
		b.reply(ctx, chatID, b.help())
		return true, false
	case "/reset":
		b.reply(ctx, chatID, "Conversation cleared.")
		return true, true
	case "/task":
		b.task(ctx, work, chatID, text)
		return true, false
	}
	return false, false
}

func commandName(text string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return name
}

func chatKey(chatID int64) string { return strconv.FormatInt(chatID, 10) }

func (b *Bot) help() string {
	help := "Send me a message and I will answer with " + b.agent.Model() + ".\n\n"
	if b.tasks != nil {
		help += "/task owner/name what to change - change a repository in its dev container and open a pull request\n"
	}
	return help +
		"/stop - stop what I am doing in this chat and drop the messages that wait\n" +
		"/reset - forget this conversation\n" +
		"/help - show this message"
}

// Telegram rejects a message over maxMessage characters, so a long answer goes
// out in parts.
func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	for _, part := range splitMessage(text, maxMessage) {
		_, err := b.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: part})
		if err != nil {
			log.Printf("sendMessage: %v", err)
			return
		}
	}
}
