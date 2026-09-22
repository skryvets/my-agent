package session

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

// fakeAgent answers with what the test set and records every history it saw.
type fakeAgent struct {
	answer func(history agent.History) (string, error)

	mu   sync.Mutex
	seen []agent.History
}

func (f *fakeAgent) Model() string { return "test-model" }

func (f *fakeAgent) Chat(ctx context.Context, history agent.History, stream io.Writer) (string, error) {
	f.mu.Lock()
	f.seen = append(f.seen, append(agent.History(nil), history...))
	f.mu.Unlock()

	answer, err := f.answer(history)
	if err == nil {
		io.WriteString(stream, answer)
	}
	return answer, err
}

func (f *fakeAgent) histories() []agent.History {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agent.History(nil), f.seen...)
}

// fakeTasks records the run it was asked for and reports back.
type fakeTasks struct {
	chat        string
	repository  string
	instruction string
	lines       []string
	err         error
	lost        []task.Run
}

func (f *fakeTasks) Start(ctx context.Context, chat, repository, instruction string, report task.Report) error {
	f.chat, f.repository, f.instruction = chat, repository, instruction
	for _, line := range f.lines {
		report(line)
	}
	return f.err
}

func (f *fakeTasks) Interrupted(ctx context.Context) []task.Run { return f.lost }

// newSession is a session whose replies land in the returned slice.
func newSession(t *testing.T, answer func(agent.History) (string, error)) (*Session, *[]string, *fakeAgent) {
	t.Helper()
	var replies []string
	model := &fakeAgent{answer: answer}
	s := &Session{
		Agent: model,
		Key:   "99",
		Reply: func(text string) { replies = append(replies, text) },
	}
	return s, &replies, model
}

func answerWith(text string) func(agent.History) (string, error) {
	return func(agent.History) (string, error) { return text, nil }
}
