package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ergochat/readline"

	"github.com/skryvets/my-agent/internal/agent"
)

type fakeAgent struct {
	answer func(agent.History) (agent.Message, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, stream io.Writer) (agent.Message, error) {
	fmt.Fprint(stream, "answer")
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
	if len(seen[1]) != 3 || seen[1][0].Content != "first" || seen[1][2].Content != "second" {
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
	if len(seen[1]) != 1 || seen[1][0].Content != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
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

	model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
		return agent.Message{Content: "answer"}, nil
	}}
	if err := Run(context.Background(), model); err != nil {
		t.Fatalf("Run: %v", err)
	}

	printed, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(printed), "test-model") {
		t.Errorf("banner = %q", printed)
	}
}

// newTestEditor builds a readline instance over plain readers and writers, so
// the interactive session runs without a terminal.
func newTestEditor(t *testing.T, in io.Reader, out io.Writer) *readline.Instance {
	t.Helper()
	editor, err := readline.NewFromConfig(&readline.Config{
		Prompt:         prompt,
		Stdin:          in,
		Stdout:         out,
		Stderr:         out,
		FuncIsTerminal: func() bool { return true },
		FuncMakeRaw:    func() error { return nil },
		FuncExitRaw:    func() error { return nil },
		FuncGetSize:    func() (int, int) { return 80, 24 },
	})
	if err != nil {
		t.Fatalf("readline: %v", err)
	}
	t.Cleanup(func() { editor.Close() })
	return editor
}

func TestInteractiveAnswersAndKeepsHistory(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return agent.Message{Content: "answer"}, nil
	}}

	var out strings.Builder
	editor := newTestEditor(t, strings.NewReader("first\n   \nsecond\n"), &out)
	chat := &session{model: model}
	if err := chat.runInteractive(context.Background(), editor); err != nil {
		t.Fatalf("interactive: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("model called %d times, want 2", len(seen))
	}
	if len(seen[1]) != 3 || seen[1][0].Content != "first" || seen[1][2].Content != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if !strings.Contains(out.String(), "test-model") {
		t.Errorf("banner does not name the model: %q", out.String())
	}
}

func TestInteractiveReportsFailedTurnAndDropsIt(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (agent.Message, error) {
		seen = append(seen, append(agent.History(nil), history...))
		if len(seen) == 1 {
			return agent.Message{}, errors.New("rate limited")
		}
		return agent.Message{Content: "ok"}, nil
	}}

	var out strings.Builder
	editor := newTestEditor(t, strings.NewReader("boom\nretry\n"), &out)
	chat := &session{model: model}
	if err := chat.runInteractive(context.Background(), editor); err != nil {
		t.Fatalf("interactive: %v", err)
	}

	if !strings.Contains(out.String(), "rate limited") {
		t.Errorf("output = %q", out.String())
	}
	if len(seen[1]) != 1 || seen[1][0].Content != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestInteractiveEndsOnInterruptAndEndOfInput(t *testing.T) {
	for name, input := range map[string]string{"interrupt": "\x03", "end of input": ""} {
		t.Run(name, func(t *testing.T) {
			called := 0
			model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
				called++
				return agent.Message{Content: "answer"}, nil
			}}

			var out strings.Builder
			editor := newTestEditor(t, strings.NewReader(input), &out)
			chat := &session{model: model}
			if err := chat.runInteractive(context.Background(), editor); err != nil {
				t.Fatalf("interactive: %v", err)
			}
			if called != 0 {
				t.Errorf("model was called %d times", called)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	null, err := os.Open("/dev/null")
	if err != nil {
		t.Skip("no /dev/null")
	}
	defer null.Close()
	if !isTerminal(null) {
		t.Error("/dev/null is not a terminal")
	}

	plain, err := os.CreateTemp(t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if isTerminal(plain) {
		t.Error("a regular file is a terminal")
	}
}
