package telegram

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

// slowAgent answers "slow" only when it is stopped, and anything else at once.
type slowAgent struct {
	started chan struct{}

	mu    sync.Mutex
	asked []string
}

func (s *slowAgent) Model() string { return "test-model" }

func (s *slowAgent) Chat(ctx context.Context, history agent.History, _ io.Writer) (string, error) {
	text := history[len(history)-1].Content
	s.mu.Lock()
	s.asked = append(s.asked, text)
	s.mu.Unlock()

	if text != "slow" {
		return "answer to " + text, nil
	}
	close(s.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func (s *slowAgent) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

func TestStopEndsTheTurnAndDropsTheQueue(t *testing.T) {
	fake := newFakeTelegram(t)
	model := &slowAgent{started: make(chan struct{})}
	bot := newTestBot(fake, nil)
	bot.agent = model

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "slow"))
	<-model.started
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "queued"))
	bot.dispatch(ctx, nil, textUpdate(3, 42, 99, "/stop"))
	if got := fake.nextSent(t); got != "Stopped." {
		t.Fatalf("reply = %q", got)
	}

	bot.dispatch(ctx, nil, textUpdate(4, 42, 99, "hello"))
	if got := fake.nextSent(t); got != "answer to hello" {
		t.Fatalf("reply = %q, want no report of the stopped turn", got)
	}
	if asked := model.seen(); strings.Join(asked, ",") != "slow,hello" {
		t.Errorf("the model was asked %#v, want the queued message dropped", asked)
	}
}

func TestStopEndsATask(t *testing.T) {
	fake := newFakeTelegram(t)
	tasks := &fakeTasks{started: make(chan struct{})}
	bot := newTestBot(fake, nil)
	bot.tasks = tasks

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/task skryvets/my-agent fix it"))
	<-tasks.started
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "/stop"))
	if got := fake.nextSent(t); got != "Stopped." {
		t.Fatalf("reply = %q", got)
	}
	fake.expectNoSend(t)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		bot.mu.Lock()
		busy := len(bot.working)
		bot.mu.Unlock()
		if busy == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("the chat stayed busy after the task stopped")
}

func TestStopWithNothingToStop(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)
	bot.allowed = map[int64]bool{42: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 7, 99, "/stop"))
	fake.expectNoSend(t)

	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "/stop"))
	if got := fake.nextSent(t); got != "Nothing to stop." {
		t.Errorf("reply = %q", got)
	}
}

func TestStopDropsAQueueThatWaits(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)
	queue := make(chan string, queueSize)
	queue <- "waiting"
	bot.queues[99] = queue

	bot.stop(context.Background(), 99)
	if got := fake.nextSent(t); got != "Stopped." {
		t.Errorf("reply = %q", got)
	}
	if len(queue) != 0 {
		t.Errorf("%d messages still wait", len(queue))
	}
}
