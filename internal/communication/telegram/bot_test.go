package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

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

func TestRetryDelayPrefersRetryAfter(t *testing.T) {
	err := &Error{RetryAfter: 12}
	if got := retryDelay(err, time.Second); got != 12*time.Second {
		t.Errorf("got %v", got)
	}
	if got := retryDelay(errors.New("boom"), 4*time.Second); got != 4*time.Second {
		t.Errorf("got %v", got)
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
