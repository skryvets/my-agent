package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
)

type fakeAgent struct {
	answer func(agent.History) (agent.Message, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, _ io.Writer) (agent.Message, error) {
	return f.answer(history)
}

func TestRunKeepsHistoryAndSkipsBlankLines(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return agent.Message{Content: "answer"}, nil
	}}

	var out, errOut strings.Builder
	in := strings.NewReader("first\n   \nsecond\n")
	if err := run(context.Background(), model, in, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("model called %d times, want 2", len(seen))
	}
	if len(seen[1]) != 3 || seen[1][0]["content"] != "first" || seen[1][2]["content"] != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if !strings.Contains(out.String(), "test-model") {
		t.Errorf("banner does not name the model: %q", out.String())
	}
}

func TestRunReportsFailedTurnAndDropsIt(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		if len(seen) == 1 {
			return agent.Message{}, errors.New("rate limited")
		}
		return agent.Message{Content: "ok"}, nil
	}}

	var out, errOut strings.Builder
	if err := run(context.Background(), model, strings.NewReader("boom\nretry\n"), &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(errOut.String(), "rate limited") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if len(seen[1]) != 1 || seen[1][0]["content"] != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}
