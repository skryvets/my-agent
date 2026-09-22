package task

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/conversation"
	"github.com/skryvets/my-agent/internal/devcontainer"
)

// fakeAgent stands in for the model. It writes into the checkout the way the
// real one would through its tools, and records the conversation it was given.
type fakeAgent struct {
	// answer is what every turn replies. answers, when it is set, replies
	// one line for each turn in order: the work, then the commit subject.
	answer  string
	answers []string
	err     error
	write   func(dir string)
	box     *fakeSandbox

	mu   sync.Mutex
	keys []string
	told []string
}

func (f *fakeAgent) Chat(ctx context.Context, history agent.History, _ io.Writer) (string, error) {
	f.mu.Lock()
	f.keys = append(f.keys, conversation.KeyOf(ctx))
	if len(history) > 0 {
		text := history[0].Content
		f.told = append(f.told, text)
	}
	f.mu.Unlock()

	if f.write != nil {
		f.write(f.checkout())
	}
	if f.err != nil {
		return "", f.err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	answer := f.answer
	if len(f.answers) > 0 {
		answer = f.answers[0]
		f.answers = f.answers[1:]
	}
	return answer, nil
}

// checkout is the directory the container of this run was bound to.
func (f *fakeAgent) checkout() string {
	if f.box == nil {
		return ""
	}
	bound, _ := f.box.seen()
	for _, config := range bound {
		return config.Root
	}
	return ""
}

func (f *fakeAgent) seen() (keys, told []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...), append([]string(nil), f.told...)
}

// fakeSandbox records the container a run asked for and the commands it ran
// there.
type fakeSandbox struct {
	mu     sync.Mutex
	bound  map[string]devcontainer.Config
	closed []string
	ran    [][]string
	err    error
	// exit answers a command whose first argument it names with that code.
	exit    map[string]int
	execErr error
}

func newFakeSandbox() *fakeSandbox {
	return &fakeSandbox{bound: map[string]devcontainer.Config{}}
}

func (f *fakeSandbox) Bind(ctx context.Context, key string, config devcontainer.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.bound[key] = config
	return nil
}

func (f *fakeSandbox) Exec(ctx context.Context, args []string) (string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, append([]string{conversation.KeyOf(ctx)}, args...))
	if f.execErr != nil {
		return "", 0, f.execErr
	}
	if code := f.exit[args[0]]; code != 0 {
		return "the setup broke", code, nil
	}
	return "", 0, nil
}

func (f *fakeSandbox) commands() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.ran...)
}

func (f *fakeSandbox) Close(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, key)
	return nil
}

func (f *fakeSandbox) seen() (bound map[string]devcontainer.Config, closed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bound = map[string]devcontainer.Config{}
	for key, config := range f.bound {
		bound[key] = config
	}
	return bound, append([]string(nil), f.closed...)
}

// reporter collects the progress lines a run sent to the chat.
type reporter struct {
	mu    sync.Mutex
	lines []string
}

func (r *reporter) report(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}

func (r *reporter) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// writeInto is the change a model would make.
func writeInto(t *testing.T, name, content string) func(string) {
	t.Helper()
	return func(dir string) {
		if dir == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Errorf("writing %s: %v", name, err)
		}
	}
}
