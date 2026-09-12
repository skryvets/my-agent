package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/approval"
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
	chat := &session{model: model, in: in, out: &out, errOut: &errOut}
	if err := chat.run(context.Background()); err != nil {
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
	chat := &session{model: model, in: strings.NewReader("boom\nretry\n"), out: &out, errOut: &errOut}
	if err := chat.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(errOut.String(), "rate limited") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if len(seen[1]) != 1 || seen[1][0]["content"] != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

// recordingApprovals keeps the handler the session registered.
type recordingApprovals struct {
	ask approval.Ask
}

func (r *recordingApprovals) Handle(ask approval.Ask) { r.ask = ask }

func TestSessionAsksOnTheSameInput(t *testing.T) {
	approvals := &recordingApprovals{}
	var answers []bool

	model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
		// A tool asks while Chat runs, which is when the loop above is
		// blocked and the input is free.
		allowed, err := approvals.ask(context.Background(), "", approval.Request{
			Tool:    "shell",
			Details: "curl https://example.com",
		})
		if err != nil {
			t.Errorf("ask: %v", err)
		}
		answers = append(answers, allowed)
		return agent.Message{Content: "answer"}, nil
	}}

	var out, errOut strings.Builder
	chat := &session{
		model:     model,
		approvals: approvals,
		in:        strings.NewReader("first\ny\nsecond\nno\nthird\n"),
		out:       &out,
		errOut:    &errOut,
	}
	if err := chat.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(answers) != 3 {
		t.Fatalf("answers = %#v", answers)
	}
	if !answers[0] || answers[1] || answers[2] {
		t.Errorf("answers = %#v, want yes, no, then no for the end of input", answers)
	}
	if !strings.Contains(out.String(), "may I run this?") {
		t.Errorf("the question was not printed: %q", out.String())
	}
	if !strings.Contains(out.String(), "curl https://example.com") {
		t.Errorf("the question does not say what would run: %q", out.String())
	}
}

func TestRunReadsTheRealStandardStreams(t *testing.T) {
	in, questions, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, out
	t.Cleanup(func() { os.Stdin, os.Stdout = oldIn, oldOut })

	io.WriteString(questions, "hello\n")
	questions.Close()

	approvals := &recordingApprovals{}
	model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
		return agent.Message{Content: "answer"}, nil
	}}
	if err := Run(context.Background(), model, WithApproval(approvals)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if approvals.ask == nil {
		t.Error("the session did not offer to answer the questions")
	}

	printed, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(printed), "test-model") {
		t.Errorf("banner = %q", printed)
	}
}
