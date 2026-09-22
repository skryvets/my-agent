package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

// fakeTasks records the run the bot asked for and reports back.
type fakeTasks struct {
	mu          sync.Mutex
	chat        string
	repository  string
	instruction string
	lines       []string
	err         error
	lost        []task.Run
	// started, when it is set, is closed when a run begins, and the run then
	// lasts until it is stopped.
	started chan struct{}
}

func (f *fakeTasks) Start(ctx context.Context, chat, repository, instruction string, report task.Report) error {
	f.mu.Lock()
	f.chat, f.repository, f.instruction = chat, repository, instruction
	f.mu.Unlock()

	for _, line := range f.lines {
		report(line)
	}
	if f.started != nil {
		close(f.started)
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func (f *fakeTasks) Interrupted(ctx context.Context) []task.Run { return f.lost }

func (f *fakeTasks) seen() (chat, repository, instruction string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chat, f.repository, f.instruction
}

func TestTaskStartsARunAndReportsItsProgress(t *testing.T) {
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
	if chat != "99" || repository != "skryvets/my-agent" {
		t.Errorf("run of %q in chat %q", repository, chat)
	}
	if instruction != "fix the lint warning" {
		t.Errorf("instruction = %q", instruction)
	}
}

func TestTaskAnswersWhenItCannotRun(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No runner was wired in.
	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/task skryvets/my-agent fix it"))
	if got := fake.nextSent(t); !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("reply = %q", got)
	}

	bot.tasks = &fakeTasks{}
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "/task"))
	if got := fake.nextSent(t); !strings.Contains(got, "/task owner/name") {
		t.Errorf("reply = %q", got)
	}
	bot.dispatch(ctx, nil, textUpdate(3, 42, 99, "/task skryvets/my-agent"))
	if got := fake.nextSent(t); !strings.Contains(got, "/task owner/name") {
		t.Errorf("reply = %q", got)
	}
}

func TestTaskReportsAFailedRun(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)
	bot.tasks = &fakeTasks{err: errors.New("github said no")}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/task skryvets/my-agent fix it"))
	if got := fake.nextSent(t); !strings.Contains(got, "github said no") {
		t.Errorf("reply = %q", got)
	}
}

func TestTaskKeepsTheConversationGoing(t *testing.T) {
	fake := newFakeTelegram(t)
	var asked []agent.History
	bot := newTestBot(fake, func(history agent.History) (string, error) {
		asked = append(asked, history)
		return "answer", nil
	})
	bot.tasks = &fakeTasks{lines: []string{"Cloning"}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/task skryvets/my-agent fix it"))
	fake.nextSent(t)
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "what did you do?"))
	fake.nextSent(t)

	// The command is not a turn of the conversation.
	if len(asked) != 1 || len(asked[0]) != 1 {
		t.Errorf("history = %#v", asked)
	}
}

func TestReportInterruptedTellsEachChat(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)
	bot.tasks = &fakeTasks{lost: []task.Run{
		{ID: "1", Chat: "99", Repo: "skryvets/my-agent", Instruction: "fix it", Branch: "my-agent/1", State: task.Interrupted},
		{ID: "2", Chat: "not-a-chat", Repo: "a/b"},
	}}

	bot.reportInterrupted(context.Background())

	got := fake.nextSent(t)
	if !strings.Contains(got, "skryvets/my-agent") || !strings.Contains(got, "my-agent/1") {
		t.Errorf("reply = %q", got)
	}
	fake.expectNoSend(t)
}

func TestHelpNamesTaskOnlyWhenItCanRun(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, nil, textUpdate(1, 42, 99, "/help"))
	if got := fake.nextSent(t); strings.Contains(got, "/task") {
		t.Errorf("help offers a command that cannot run: %q", got)
	}

	bot.tasks = &fakeTasks{}
	bot.dispatch(ctx, nil, textUpdate(2, 42, 99, "/help"))
	if got := fake.nextSent(t); !strings.Contains(got, "/task owner/name") {
		t.Errorf("help = %q", got)
	}
}
