package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/skryvets/my-agent/internal/approval"
)

// asked counts the questions of this process, so each one has an id short
// enough for callback_data, which the Bot API caps at 64 bytes.
var asked atomic.Int64

// ask puts one tool call to the chat it belongs to and waits for a button.
// The agent loop is blocked while it waits, which is the point.
func (b *Bot) ask(ctx context.Context, key string, request approval.Request) (bool, error) {
	chatID, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return false, fmt.Errorf("no chat to ask: %q", key)
	}

	id := strconv.FormatInt(asked.Add(1), 10)
	answer := make(chan bool, 1)

	b.mu.Lock()
	b.waiting[id] = answer
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.waiting, id)
		b.mu.Unlock()
	}()

	text := fmt.Sprintf("May I run this?\n\n%s\n%s", request.Tool, request.Details)
	buttons := [][]button{{
		{Text: "Approve", Data: "y:" + id},
		{Text: "Deny", Data: "n:" + id},
	}}
	messageID, err := b.client.sendKeyboard(ctx, chatID, text, buttons)
	if err != nil {
		return false, err
	}

	select {
	case <-ctx.Done():
		b.close(context.WithoutCancel(ctx), chatID, messageID, text, "no answer, so no")
		return false, ctx.Err()
	case allowed := <-answer:
		decision := "denied"
		if allowed {
			decision = "approved"
		}
		b.close(ctx, chatID, messageID, text, decision)
		return allowed, nil
	}
}

// answer hands a pressed button to the call that is waiting for it.
func (b *Bot) answer(ctx context.Context, query *callbackQuery) {
	if len(b.allowed) > 0 && !b.allowed[query.From.ID] {
		log.Printf("ignoring a button from user %d (@%s)", query.From.ID, query.From.Username)
		return
	}

	decision, id, found := strings.Cut(query.Data, ":")
	if !found {
		return
	}

	b.mu.Lock()
	waiting, ok := b.waiting[id]
	b.mu.Unlock()

	note := "That question is no longer open."
	if ok {
		note = "Denied."
		if decision == "y" {
			note = "Approved."
		}
		waiting <- decision == "y"
	}
	if err := b.client.answerCallback(ctx, query.ID, note); err != nil {
		log.Printf("answerCallbackQuery: %v", err)
	}
}

// close replaces the question with what was decided, which also takes the
// buttons away so nobody answers it twice.
func (b *Bot) close(ctx context.Context, chatID, messageID int64, text, decision string) {
	if err := b.client.editMessage(ctx, chatID, messageID, text+"\n\n-> "+decision); err != nil {
		log.Printf("editMessageText: %v", err)
	}
}
