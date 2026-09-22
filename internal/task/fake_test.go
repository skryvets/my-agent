package task

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/devcontainer"
	"github.com/skryvets/my-agent/internal/tools"
)

// fakeAgent stands in for the model. It writes into the checkout the way the
// real one would through its tools, and records the conversation it was given.
type fakeAgent struct {
	// answer is what every turn replies.
	answer string
	err    error
	write  func(dir string)
	box    *fakeSandbox

	mu   sync.Mutex
	told []string
}

func (f *fakeAgent) Chat(ctx context.Context, history agent.History, _ io.Writer) (string, error) {
	f.mu.Lock()
	if len(history) > 0 {
		f.told = append(f.told, history[0].Content)
	}
	f.mu.Unlock()

	if f.write != nil {
		f.write(f.checkout())
	}
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

// checkout is the directory the container of this run was started on.
func (f *fakeAgent) checkout() string {
	if f.box == nil {
		return ""
	}
	for _, box := range f.box.containers() {
		return box.config.Root
	}
	return ""
}

func (f *fakeAgent) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.told...)
}

// fakeSandbox records the containers a run asked for.
type fakeSandbox struct {
	mu      sync.Mutex
	started []*fakeContainer
	err     error
	// exit answers a command whose first argument it names with that code.
	exit    map[string]int
	execErr error
}

func (f *fakeSandbox) Start(ctx context.Context, name string, config devcontainer.Config) (Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	box := &fakeContainer{name: name, config: config, sandbox: f}
	f.started = append(f.started, box)
	return box, nil
}

func (f *fakeSandbox) containers() []*fakeContainer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeContainer(nil), f.started...)
}

// fakeContainer records the commands a run ran in it and whether it was
// removed. The tools of a run are bound to it, but the fake agent writes to
// the checkout on the host instead, so the Workspace methods are never used.
type fakeContainer struct {
	name    string
	config  devcontainer.Config
	sandbox *fakeSandbox

	mu      sync.Mutex
	ran     [][]string
	removed int
}

var _ tools.Workspace = (*fakeContainer)(nil)

func (f *fakeContainer) Run(ctx context.Context, command string) (string, error) {
	return "", nil
}

func (f *fakeContainer) ReadFile(ctx context.Context, name string) (string, error) {
	return "", nil
}

func (f *fakeContainer) WriteFile(ctx context.Context, name, content string) error {
	return nil
}

func (f *fakeContainer) Exec(ctx context.Context, args []string) (string, int, error) {
	f.mu.Lock()
	f.ran = append(f.ran, args)
	f.mu.Unlock()

	f.sandbox.mu.Lock()
	defer f.sandbox.mu.Unlock()
	if f.sandbox.execErr != nil {
		return "", 0, f.sandbox.execErr
	}
	if code := f.sandbox.exit[args[0]]; code != 0 {
		return "the setup broke", code, nil
	}
	return "", 0, nil
}

func (f *fakeContainer) Remove(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed++
	return nil
}

func (f *fakeContainer) commands() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.ran...)
}

func (f *fakeContainer) gone() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removed == 1
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
