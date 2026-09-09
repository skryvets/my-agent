package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

type fakeAgent struct {
	answer func(agent.History) (agent.Message, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, _ io.Writer) (agent.Message, error) {
	return f.answer(history)
}

func newTestBot(fake *fakeTelegram, answer func(agent.History) (agent.Message, error)) *Bot {
	return &Bot{
		client:    fake.client(),
		agent:     fakeAgent{answer: answer},
		retryBase: time.Millisecond,
		sessions:  map[int64]chan string{},
	}
}

func textUpdate(id, userID, chatID int64, text string) update {
	msg := &message{Text: text}
	msg.MessageID = id
	msg.From.ID = userID
	msg.Chat.ID = chatID
	msg.Chat.Type = "private"
	return update{UpdateID: id, Message: msg}
}

func TestServeAnswersWithHistoryAndTypingAction(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return agent.Message{Content: fmt.Sprintf("answer %d", len(seen))}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "first"))
	if got := fake.nextSent(t); got != "answer 1" {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "second"))
	if got := fake.nextSent(t); got != "answer 2" {
		t.Fatalf("reply = %q", got)
	}

	if len(seen) != 2 || len(seen[1]) != 3 {
		t.Fatalf("history = %#v", seen)
	}
	if seen[1][0]["content"] != "first" || seen[1][1]["content"] != "answer 1" || seen[1][2]["content"] != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if seen[1][1]["role"] != "assistant" {
		t.Errorf("assistant turn = %#v", seen[1][1])
	}

	methods := fake.methods()
	if methods[0] != "sendChatAction" {
		t.Errorf("methods = %v", methods)
	}
	if action := fake.calls[0].Payload["action"]; action != "typing" {
		t.Errorf("action = %#v", action)
	}
}

func TestServeReportsFailedTurnAndDropsIt(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		if len(seen) == 1 {
			return agent.Message{}, errors.New("rate limited")
		}
		return agent.Message{Content: "ok"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "boom"))
	if got := fake.nextSent(t); !strings.Contains(got, "rate limited") {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "retry"))
	if got := fake.nextSent(t); got != "ok" {
		t.Fatalf("reply = %q", got)
	}
	if len(seen[1]) != 1 || seen[1][0]["content"] != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestServeHandlesCommands(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen []agent.History
	bot := newTestBot(fake, func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return agent.Message{Content: "answer"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "/start"))
	got := fake.nextSent(t)
	if !strings.Contains(got, "/reset") {
		t.Fatalf("start reply = %q", got)
	}
	if !strings.Contains(got, "test-model") {
		t.Errorf("help does not name the model: %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "hello"))
	fake.nextSent(t)
	bot.dispatch(ctx, textUpdate(3, 42, 99, "/reset"))
	if got := fake.nextSent(t); got != "Conversation cleared." {
		t.Fatalf("reset reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(4, 42, 99, "again"))
	fake.nextSent(t)

	if len(seen) != 2 || len(seen[1]) != 1 {
		t.Errorf("history was not cleared: %#v", seen)
	}
}

func TestDispatchIgnoresNonTextAndDisallowedUsers(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, func(agent.History) (agent.Message, error) {
		t.Error("model should not be called")
		return agent.Message{}, nil
	})
	bot.allowed = map[int64]bool{7: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, update{UpdateID: 1})
	bot.dispatch(ctx, textUpdate(2, 7, 99, "   "))
	bot.dispatch(ctx, textUpdate(3, 42, 99, "let me in"))
	fake.expectNoSend(t)
}

func TestDispatchTellsSenderWhenQueueIsFull(t *testing.T) {
	fake := newFakeTelegram(t)
	release := make(chan struct{})
	bot := newTestBot(fake, func(agent.History) (agent.Message, error) {
		<-release
		return agent.Message{Content: "done"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(release)

	for id := int64(1); id <= queueSize+2; id++ {
		bot.dispatch(ctx, textUpdate(id, 42, 99, "queued"))
	}
	if got := fake.nextSent(t); !strings.Contains(got, "still working") {
		t.Fatalf("reply = %q", got)
	}
}

func TestRunTracksOffsetAndStopsOnContextCancel(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":true,"result":[{"update_id":7,"message":{"message_id":1,"from":{"id":42},"chat":{"id":99,"type":"private"},"text":"/help"}}]}`)
	bot := newTestBot(fake, func(agent.History) (agent.Message, error) {
		return agent.Message{Content: "unused"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bot.run(ctx) }()

	fake.nextSent(t)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.calls[0].Payload["offset"] != nil {
		t.Errorf("first poll should have no offset: %#v", fake.calls[0].Payload)
	}
	for _, call := range fake.calls {
		if call.Method == "getUpdates" && call.Payload["offset"] != nil && call.Payload["offset"] != float64(8) {
			t.Errorf("offset = %#v, want 8", call.Payload["offset"])
		}
	}
}

func TestParseAllowedUsers(t *testing.T) {
	allowed, err := parseAllowedUsers(" 42, 7 ,")
	if err != nil {
		t.Fatalf("parseAllowedUsers: %v", err)
	}
	if !reflect.DeepEqual(allowed, map[int64]bool{42: true, 7: true}) {
		t.Errorf("allowed = %#v", allowed)
	}
	if empty, err := parseAllowedUsers(""); err != nil || len(empty) != 0 {
		t.Errorf("empty = %#v, err = %v", empty, err)
	}
	if _, err := parseAllowedUsers("sergey"); err == nil {
		t.Error("expected error for a non-numeric id")
	}
}

func TestRetryDelayPrefersRetryAfter(t *testing.T) {
	err := &Error{RetryAfter: 12}
	if got := retryDelay(err, time.Second); got != 12*time.Second {
		t.Errorf("got %v", got)
	}
	if got := retryDelay(errors.New("boom"), 4*time.Second); got != 4*time.Second {
		t.Errorf("got %v", got)
	}
}

func TestRunRequiresToken(t *testing.T) {
	model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
		return agent.Message{}, nil
	}}

	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if err := Run(context.Background(), model); err == nil {
		t.Fatal("expected an error without a bot token")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_ALLOWED_USERS", "nope")
	if err := Run(context.Background(), model); err == nil {
		t.Fatal("expected an error for a bad allowlist")
	}
}

func TestRunRetriesAfterAPollFailure(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":false,"error_code":500,"description":"internal"}`)
	bot := newTestBot(fake, func(agent.History) (agent.Message, error) {
		return agent.Message{Content: "hi"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	fake.reply("getUpdates", `{"ok":true,"result":[{"update_id":3,"message":{"message_id":1,"from":{"id":42},"chat":{"id":99,"type":"private"},"text":"/help"}}]}`)
	fake.nextSent(t)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	polls := 0
	for _, call := range fake.calls {
		if call.Method == "getUpdates" {
			polls++
		}
	}
	if polls < 2 {
		t.Errorf("polls = %d, want the failed poll to be retried", polls)
	}
}

func TestSleepStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Error("sleep should report a cancelled context")
	}
	if !sleep(context.Background(), time.Millisecond) {
		t.Error("sleep should report a completed wait")
	}
}
