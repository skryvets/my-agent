package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/task"
)

func TestTaskStartsARunAndReportsItsProgress(t *testing.T) {
	s, replies, model := newSession(t, answerWith("answer"))
	tasks := &fakeTasks{lines: []string{"Cloning skryvets/my-agent", "Pull request open: https://example.com/pull/7"}}
	s.Tasks = tasks

	s.Handle(context.Background(), "/task skryvets/my-agent fix the lint warning")

	if got := strings.Join(*replies, "|"); got != "Cloning skryvets/my-agent|Pull request open: https://example.com/pull/7" {
		t.Errorf("replies = %q", got)
	}
	if tasks.chat != "99" || tasks.repository != "skryvets/my-agent" || tasks.instruction != "fix the lint warning" {
		t.Errorf("run = %#v", tasks)
	}

	// The command is not a turn of the conversation.
	s.Handle(context.Background(), "what did you do?")
	if seen := model.histories(); len(seen) != 1 || len(seen[0]) != 1 {
		t.Errorf("history = %#v", seen)
	}
}

func TestTaskReadsARepositoryFollowedByALineBreak(t *testing.T) {
	s, _, _ := newSession(t, nil)
	tasks := &fakeTasks{}
	s.Tasks = tasks

	s.Handle(context.Background(), "/task skryvets/my-agent\n\nI need the docs\nchecked against the code")

	if tasks.repository != "skryvets/my-agent" || tasks.instruction != "I need the docs\nchecked against the code" {
		t.Errorf("run = %#v", tasks)
	}
}

func TestTaskAnswersWhenItCannotRun(t *testing.T) {
	s, replies, _ := newSession(t, nil)
	ctx := context.Background()

	// No runner was wired in.
	s.Handle(ctx, "/task skryvets/my-agent fix it")
	if got := (*replies)[0]; !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("reply = %q", got)
	}

	s.Tasks = &fakeTasks{}
	for _, text := range []string{"/task", "/task skryvets/my-agent"} {
		s.Handle(ctx, text)
		if got := (*replies)[len(*replies)-1]; !strings.Contains(got, "/task owner/name") {
			t.Errorf("reply to %q = %q", text, got)
		}
	}
}

func TestTaskReportsAFailedRun(t *testing.T) {
	s, replies, _ := newSession(t, nil)
	s.Tasks = &fakeTasks{err: errors.New("github said no")}

	s.Handle(context.Background(), "/task skryvets/my-agent fix it")
	if got := (*replies)[0]; !strings.Contains(got, "github said no") {
		t.Errorf("reply = %q", got)
	}
}

func TestTaskKeepsQuietAboutAStoppedRun(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()
	s, replies, _ := newSession(t, nil)
	s.Tasks = &fakeTasks{err: context.Canceled}

	s.Handle(ctx, "/task skryvets/my-agent fix it")
	if len(*replies) != 0 {
		t.Errorf("a stopped run was reported: %q", *replies)
	}
}

func TestLostSaysWhatARestartDropped(t *testing.T) {
	got := Lost(task.Run{Repo: "skryvets/my-agent", Instruction: "fix it", Branch: "my-agent/1", State: task.Interrupted})
	for _, want := range []string{"skryvets/my-agent", "fix it", "my-agent/1", "interrupted"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q does not say %q", got, want)
		}
	}
}
