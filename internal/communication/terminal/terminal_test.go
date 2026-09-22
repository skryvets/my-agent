package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

type fakeAgent struct {
	answer func(agent.History) (string, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, stream io.Writer) (string, error) {
	answer, err := f.answer(history)
	io.WriteString(stream, answer)
	return answer, err
}

// fakeTasks records the run the terminal asked for.
type fakeTasks struct {
	chat, repository, instruction string
	lost                          []task.Run
}

func (f *fakeTasks) Start(ctx context.Context, chat, repository, instruction string, report task.Report) error {
	f.chat, f.repository, f.instruction = chat, repository, instruction
	report("Cloning " + repository)
	return nil
}

func (f *fakeTasks) Interrupted(ctx context.Context) []task.Run { return f.lost }

func TestRunKeepsHistoryAndSkipsBlankLines(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (string, error) {
		seen = append(seen, append(agent.History(nil), history...))
		return "answer", nil
	}}

	var out strings.Builder
	in := strings.NewReader("first\n   \nsecond\n")
	if err := run(context.Background(), model, nil, in, &out); err != nil {
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
	if strings.Count(out.String(), "answer") != 2 {
		t.Errorf("the answer was not streamed once per turn: %q", out.String())
	}
}

func TestRunReportsFailedTurnAndDropsIt(t *testing.T) {
	var seen []agent.History
	model := fakeAgent{answer: func(history agent.History) (string, error) {
		seen = append(seen, append(agent.History(nil), history...))
		if len(seen) == 1 {
			return "", errors.New("rate limited")
		}
		return "ok", nil
	}}

	var out strings.Builder
	if err := run(context.Background(), model, nil, strings.NewReader("boom\nretry\n"), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(out.String(), "rate limited") {
		t.Errorf("out = %q", out.String())
	}
	if len(seen[1]) != 1 || seen[1][0].Content != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestRunOffersTheSameCommandsAsTelegram(t *testing.T) {
	model := fakeAgent{answer: func(agent.History) (string, error) { return "answer", nil }}
	tasks := &fakeTasks{lost: []task.Run{
		{Chat: key, Repo: "skryvets/my-agent", Instruction: "fix it", Branch: "my-agent/1", State: task.Interrupted},
		{Chat: "99", Repo: "a/b"},
	}}

	var out strings.Builder
	in := strings.NewReader("/help\n/task skryvets/my-agent fix the lint warning\n/reset\n")
	if err := run(context.Background(), model, tasks, in, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	printed := out.String()
	for _, want := range []string{"/task owner/name", "Cloning skryvets/my-agent", "Conversation cleared.", "A restart stopped the task on skryvets/my-agent"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the terminal never printed %q: %s", want, printed)
		}
	}
	if strings.Contains(printed, "a/b") {
		t.Errorf("the terminal reported the run of another chat: %s", printed)
	}
	if tasks.chat != key || tasks.instruction != "fix the lint warning" {
		t.Errorf("run = %#v", tasks)
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

	model := fakeAgent{answer: func(agent.History) (string, error) { return "answer", nil }}
	if err := Run(context.Background(), model, nil); err != nil {
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
