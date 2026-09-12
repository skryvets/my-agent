package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
)

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

func TestServeNamesAndThrowsAwayTheWorkspace(t *testing.T) {
	fake := newFakeTelegram(t)
	box := &fakeSandbox{}
	var keyed string
	bot := newTestBot(fake, func(history agent.History) (agent.Message, error) {
		return agent.Message{Content: "answer"}, nil
	})
	bot.sandbox = box
	bot.agent = keyReadingAgent{key: &keyed, box: box}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "hello"))
	fake.nextSent(t)

	keys, closed := box.seen()
	if len(keys) != 1 || keys[0] != "99" {
		t.Errorf("keys = %#v, want the chat id", keys)
	}
	if len(closed) != 0 {
		t.Errorf("the workspace was closed too early: %#v", closed)
	}
	if keyed != "99" {
		t.Errorf("the chat did not reach the agent through the context: %q", keyed)
	}

	bot.dispatch(ctx, textUpdate(2, 42, 99, "/reset"))
	fake.nextSent(t)
	if _, closed := box.seen(); len(closed) != 1 || closed[0] != "99" {
		t.Errorf("closed = %#v, want the chat id", closed)
	}

	bot.dispatch(ctx, textUpdate(3, 42, 99, "/help"))
	if got := fake.nextSent(t); !strings.Contains(got, "throw away its workspace") {
		t.Errorf("help does not mention the workspace: %q", got)
	}
}

func TestServeReportsAWorkspaceThatWillNotClose(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, func(agent.History) (agent.Message, error) {
		return agent.Message{Content: "answer"}, nil
	})
	bot.sandbox = &fakeSandbox{err: errors.New("the daemon is gone")}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A workspace that will not close must still clear the conversation.
	bot.dispatch(ctx, textUpdate(1, 42, 99, "/reset"))
	if got := fake.nextSent(t); got != "Conversation cleared." {
		t.Errorf("reset reply = %q", got)
	}
}

// keyReadingAgent reports the chat key the bot put into the context.
type keyReadingAgent struct {
	key *string
	box *fakeSandbox
}

func (k keyReadingAgent) Model() string { return "test-model" }

func (k keyReadingAgent) Chat(ctx context.Context, _ agent.History, _ io.Writer) (agent.Message, error) {
	*k.key, _ = ctx.Value(k.box).(string)
	return agent.Message{Content: "answer"}, nil
}
