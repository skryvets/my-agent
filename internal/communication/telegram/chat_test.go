package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

func TestServeAnswersInOrderWithTypingAction(t *testing.T) {
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

	// The session of the chat kept the history between the two.
	if len(seen) != 2 || len(seen[1]) != 3 || seen[1][1].Content != "answer 1" {
		t.Fatalf("history = %#v", seen)
	}

	methods := fake.methods()
	if methods[0] != "sendChatAction" {
		t.Errorf("methods = %v", methods)
	}
	if action := fake.calls[0].Form["action"]; action != "typing" {
		t.Errorf("action = %#v", action)
	}
}

func TestServeAnswersWhenTheTypingActionFails(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.refuseActions = true
	bot := newTestBot(fake, func(agent.History) (string, error) { return "still here", nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "hello"))
	if got := fake.nextSent(t); got != "still here" {
		t.Errorf("reply = %q", got)
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

func TestTaskRunsInTheChatAndReportsToIt(t *testing.T) {
	fake := newFakeTelegram(t)
	tasks := &fakeTasks{lines: []string{"Cloning skryvets/my-agent", "Pull request open: https://example.com/pull/7"}}
	bot := newTestBot(fake, nil)
	bot.tasks = tasks

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/task skryvets/my-agent fix the lint warning"))

	if got := fake.nextSent(t); got != "Cloning skryvets/my-agent" {
		t.Errorf("first line = %q", got)
	}
	if got := fake.nextSent(t); !strings.Contains(got, "pull/7") {
		t.Errorf("last line = %q", got)
	}
	chat, repository, instruction := tasks.seen()
	if chat != "99" || repository != "skryvets/my-agent" || instruction != "fix the lint warning" {
		t.Errorf("run of %q in chat %q: %q", repository, chat, instruction)
	}
}

func TestReportInterruptedTellsEachChat(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)

	// With /task off there is nothing to report.
	bot.reportInterrupted(context.Background())
	fake.expectNoSend(t)

	bot.tasks = &fakeTasks{lost: []task.Run{
		{ID: "1", Chat: "99", Repo: "skryvets/my-agent", Instruction: "fix it", Branch: "my-agent/1", State: task.Interrupted},
		{ID: "2", Chat: "not-a-chat", Repo: "a/b"},
	}}

	bot.reportInterrupted(context.Background())

	got := fake.nextSent(t)
	if !strings.Contains(got, "skryvets/my-agent") || !strings.Contains(got, "my-agent/1") {
		t.Errorf("reply = %q", got)
	}
	if got := fake.calls[0].Form["chat_id"]; got != "99" {
		t.Errorf("chat_id = %q", got)
	}
	fake.expectNoSend(t)
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
}
