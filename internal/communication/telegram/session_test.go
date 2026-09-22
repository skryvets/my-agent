package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/go-telegram/bot/models"
	"github.com/skryvets/my-agent/internal/agent"
)

func TestServeAnswersWithHistoryAndTypingAction(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (string, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return fmt.Sprintf("answer %d", len(seen)), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "first"))
	if got := fake.nextSent(t); got != "answer 1" {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "second"))
	if got := fake.nextSent(t); got != "answer 2" {
		t.Fatalf("reply = %q", got)
	}

	if len(seen) != 2 || len(seen[1]) != 3 {
		t.Fatalf("history = %#v", seen)
	}
	if seen[1][0].Content != "first" || seen[1][1].Content != "answer 1" || seen[1][2].Content != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if seen[1][1].Role != schema.Assistant {
		t.Errorf("assistant turn = %#v", seen[1][1])
	}

	methods := fake.methods()
	if methods[0] != "sendChatAction" {
		t.Errorf("methods = %v", methods)
	}
	if action := fake.calls[0].Form["action"]; action != "typing" {
		t.Errorf("action = %#v", action)
	}
}

func TestServeReportsFailedTurnAndDropsIt(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (string, error) {
		seen = append(seen, append(agent.History(nil), history...))
		if len(seen) == 1 {
			return "", errors.New("rate limited")
		}
		return "ok", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "boom"))
	if got := fake.nextSent(t); !strings.Contains(got, "rate limited") {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "retry"))
	if got := fake.nextSent(t); got != "ok" {
		t.Fatalf("reply = %q", got)
	}
	if len(seen[1]) != 1 || seen[1][0].Content != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestServeHandlesCommands(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (string, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return "answer", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/start"))
	got := fake.nextSent(t)
	if !strings.Contains(got, "/reset") {
		t.Fatalf("start reply = %q", got)
	}
	if !strings.Contains(got, "test-model") {
		t.Errorf("help does not name the model: %q", got)
	}
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "hello"))
	fake.nextSent(t)
	bot.dispatch(ctx, nil, textUpdate(3, 42, 99, "/reset"))
	if got := fake.nextSent(t); got != "Conversation cleared." {
		t.Fatalf("reset reply = %q", got)
	}
	bot.dispatch(ctx, nil, textUpdate(4, 42, 99, "again"))
	fake.nextSent(t)

	if len(seen) != 2 || len(seen[1]) != 1 {
		t.Errorf("history was not cleared: %#v", seen)
	}
}

func TestDispatchIgnoresNonTextAndDisallowedUsers(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, func(agent.History) (string, error) {
		t.Error("model should not be called")
		return "", nil
	})
	bot.allowed = map[int64]bool{7: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, &models.Update{ID: 1})
	bot.dispatch(ctx, nil, textUpdate(2, 7, 99, "   "))
	bot.dispatch(ctx, nil, textUpdate(3, 42, 99, "let me in"))
	fake.expectNoSend(t)
}

func TestDispatchTellsSenderWhenQueueIsFull(t *testing.T) {
	fake := newFakeTelegram(t)
	release := make(chan struct{})
	bot := newTestBot(fake, func(agent.History) (string, error) {
		<-release
		return "done", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(release)

	for id := int64(1); id <= queueSize+2; id++ {
		bot.dispatch(ctx, nil, textUpdate(id, 42, 99, "queued"))
	}
	if got := fake.nextSent(t); !strings.Contains(got, "still working") {
		t.Fatalf("reply = %q", got)
	}
}

func TestReplySplitsLongText(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)

	bot.reply(context.Background(), 99, strings.Repeat("a", maxMessage)+"\ntail")

	if first := fake.nextSent(t); len([]rune(first)) != maxMessage {
		t.Errorf("first part length = %d", len([]rune(first)))
	}
	if second := fake.nextSent(t); second != "tail" {
		t.Errorf("second part = %q", second)
	}
	if got := fake.calls[0].Form["chat_id"]; got != "99" {
		t.Errorf("chat_id = %q", got)
	}
}
