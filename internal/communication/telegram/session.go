package telegram

import (
	"context"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/conversation"
)

const (
	// historyTurns caps how much of a conversation is replayed to the model.
	// History lives in memory only, so a restart clears it.
	historyTurns = 10

	// queueSize is how far a sender may run ahead of the answers.
	queueSize = 4
)

// dispatch drops what the bot will not answer and queues the rest.
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
		b.reply(ctx, msg.Chat.ID, "I am still working on your previous messages, try again in a moment.")
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

	// The tools and the questions they raise both belong to this chat, and
	// find it through the context.
	ctx = conversation.WithKey(ctx, chatKey(chatID))

	for {
		var text string
		select {
		case <-ctx.Done():
			return
		case text = <-queue:
		}

		if handled, cleared := b.command(ctx, chatID, text); handled {
			if cleared {
				history = nil
			}
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

// command answers a slash command, reporting whether it handled the message and
// whether the conversation should be cleared.
func (b *Bot) command(ctx context.Context, chatID int64, text string) (handled, cleared bool) {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	switch name {
	case "/start", "/help":
		b.reply(ctx, chatID, b.help())
		return true, false
	case "/reset":
		b.forget(ctx, chatID)
		b.reply(ctx, chatID, "Conversation cleared.")
		return true, true
	case "/task":
		b.task(ctx, chatID, text)
		return true, false
	}
	return false, false
}

// forget throws away the workspace of one chat, so /reset loses the files the
// agent wrote as well as the conversation.
func (b *Bot) forget(ctx context.Context, chatID int64) {
	if b.sandbox == nil {
		return
	}
	if err := b.sandbox.Close(ctx, chatKey(chatID)); err != nil {
		log.Printf("closing the workspace of chat %d: %v", chatID, err)
	}
}

func chatKey(chatID int64) string { return strconv.FormatInt(chatID, 10) }

func (b *Bot) help() string {
	reset := "/reset - forget this conversation\n"
	if b.sandbox != nil {
		reset = "/reset - forget this conversation and throw away its workspace\n"
	}
	help := "Send me a message and I will answer with " + b.agent.Model() + ".\n\n"
	if b.tasks != nil {
		help += "/task owner/name what to change - change a repository and open a pull request\n"
	}
	return help + reset + "/help - show this message"
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if err := b.client.sendMessage(ctx, chatID, text); err != nil {
		log.Printf("sendMessage: %v", err)
	}
}
