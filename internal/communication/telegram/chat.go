package telegram

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/skryvets/my-agent/internal/session"
)

// queueSize is how far a sender may run ahead of the answers.
const queueSize = 4

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

	queue := b.queue(ctx, msg.Chat.ID)
	select {
	case queue <- msg.Text:
	default:
		b.reply(ctx, msg.Chat.ID, "I am still working on your previous messages, try again in a moment. /stop drops them.")
	}
}

// Each chat gets its own queue and goroutine, so a slow answer in one chat
// does not block another. Messages inside one chat are answered in order.
func (b *Bot) queue(ctx context.Context, chatID int64) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if queue, ok := b.queues[chatID]; ok {
		return queue
	}
	queue := make(chan string, queueSize)
	b.queues[chatID] = queue
	go b.serve(ctx, chatID, queue)
	return queue
}

// serve owns the session of one chat, so no lock is needed around it. ctx
// lives as long as the bot and carries the replies; each message runs on a
// context of its own, which /stop ends.
func (b *Bot) serve(ctx context.Context, chatID int64, queue <-chan string) {
	chat := &session.Session{
		Agent: b.agent,
		Tasks: b.tasks,
		Key:   strconv.FormatInt(chatID, 10),
		Reply: func(text string) { b.reply(ctx, chatID, text) },
	}
	for {
		select {
		case <-ctx.Done():
			return
		case text := <-queue:
			b.typing(ctx, chatID)
			work, done := b.begin(ctx, chatID)
			chat.Handle(work, text)
			done()
		}
	}
}

// typing shows the person that an answer is on its way.
func (b *Bot) typing(ctx context.Context, chatID int64) {
	_, err := b.api.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	if err != nil {
		log.Printf("sendChatAction: %v", err)
	}
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

func commandName(text string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return name
}
