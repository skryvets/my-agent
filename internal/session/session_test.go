package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/skryvets/my-agent/internal/agent"
)

func TestHandleAnswersWithTheHistory(t *testing.T) {
	calls := 0
	s, replies, model := newSession(t, func(agent.History) (string, error) {
		calls++
		return fmt.Sprintf("answer %d", calls), nil
	})
	ctx := context.Background()

	s.Handle(ctx, "first")
	s.Handle(ctx, "second")

	if got := strings.Join(*replies, "|"); got != "answer 1|answer 2" {
		t.Fatalf("replies = %q", got)
	}
	seen := model.histories()
	if len(seen) != 2 || len(seen[1]) != 3 {
		t.Fatalf("history = %#v", seen)
	}
	if seen[1][0].Content != "first" || seen[1][1].Content != "answer 1" || seen[1][2].Content != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if seen[1][1].Role != schema.Assistant {
		t.Errorf("assistant turn = %#v", seen[1][1])
	}
}

func TestHandleStreamsTheAnswerInsteadOfReplying(t *testing.T) {
	s, replies, _ := newSession(t, answerWith("typed out"))
	var out strings.Builder
	s.Stream = &out

	s.Handle(context.Background(), "hello")

	if out.String() != "typed out" {
		t.Errorf("stream = %q", out.String())
	}
	if len(*replies) != 0 {
		t.Errorf("the answer was sent twice: %q", *replies)
	}
}

func TestHandleReportsAFailedTurnAndDropsIt(t *testing.T) {
	calls := 0
	s, replies, model := newSession(t, func(agent.History) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("rate limited")
		}
		return "ok", nil
	})
	ctx := context.Background()

	s.Handle(ctx, "boom")
	if got := (*replies)[0]; !strings.Contains(got, "rate limited") {
		t.Fatalf("reply = %q", got)
	}
	s.Handle(ctx, "retry")
	if seen := model.histories(); len(seen[1]) != 1 || seen[1][0].Content != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestHandleKeepsQuietAboutAStoppedTurn(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	s, replies, _ := newSession(t, func(agent.History) (string, error) {
		stop()
		return "", context.Canceled
	})

	s.Handle(ctx, "slow")
	if len(*replies) != 0 {
		t.Errorf("a stopped turn was reported: %q", *replies)
	}
}

func TestHandleTrimsTheHistory(t *testing.T) {
	s, _, model := newSession(t, answerWith("answer"))
	ctx := context.Background()

	for i := range historyTurns + 3 {
		s.Handle(ctx, fmt.Sprint("message ", i))
	}
	seen := model.histories()
	last := seen[len(seen)-1]
	// The newest turns, and the question of this turn on top.
	if len(last) != historyTurns*2+1 {
		t.Errorf("the model saw %d messages, want %d", len(last), historyTurns*2+1)
	}
	if last[0].Role != schema.User {
		t.Errorf("history starts on %#v", last[0])
	}
}

func TestHandleAnswersTheCommands(t *testing.T) {
	s, replies, model := newSession(t, answerWith("answer"))
	ctx := context.Background()

	s.Handle(ctx, "/start")
	if got := (*replies)[0]; !strings.Contains(got, "/reset") || !strings.Contains(got, "test-model") {
		t.Fatalf("start reply = %q", got)
	}
	s.Handle(ctx, "hello")
	s.Handle(ctx, "/reset")
	if got := (*replies)[2]; got != "Conversation cleared." {
		t.Fatalf("reset reply = %q", got)
	}
	s.Handle(ctx, "again")
	if seen := model.histories(); len(seen) != 2 || len(seen[1]) != 1 {
		t.Errorf("history was not cleared: %#v", seen)
	}

	s.Handle(ctx, "/stop")
	if got := (*replies)[len(*replies)-1]; got != "Nothing to stop." {
		t.Errorf("stop reply = %q", got)
	}
}

func TestHelpNamesTaskOnlyWhenItCanRun(t *testing.T) {
	s, _, _ := newSession(t, nil)
	if help := s.Help(); strings.Contains(help, "/task") {
		t.Errorf("help offers a command that cannot run: %q", help)
	}
	s.Tasks = &fakeTasks{}
	if help := s.Help(); !strings.Contains(help, "/task owner/name") {
		t.Errorf("help = %q", help)
	}
}
