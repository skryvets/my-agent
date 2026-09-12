package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Workspace is where the tools work. The host is one, a container is another,
// and a tool cannot tell them apart.
type Workspace interface {
	// Run executes one command line with sh -c and returns the combined
	// output. A command that exits non-zero is reported in the text, not as
	// an error, because the model reads it and decides what to do.
	Run(ctx context.Context, command string) (string, error)
	ReadFile(ctx context.Context, name string) (string, error)
	WriteFile(ctx context.Context, name, content string) error
}

// Host is the machine the agent runs on. It has no isolation: use it for a
// terminal chat on a machine you own, and a sandbox for anything else.
type Host struct {
	// Dir is the working directory and the root every path is resolved
	// against. An empty Dir means the current directory.
	Dir string
}

func (h Host) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = h.Dir
	output, err := cmd.CombinedOutput()

	report := string(output)
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	if err != nil {
		report += fmt.Sprintf("\n[%v]", err)
	}
	return report, nil
}

func (h Host) ReadFile(ctx context.Context, name string) (string, error) {
	path, err := h.resolve(name)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	return string(content), err
}

func (h Host) WriteFile(ctx context.Context, name, content string) error {
	path, err := h.resolve(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// resolve keeps a path inside the root, so a model that asks for ../../etc
// gets an error instead of the file. A container needs no such guard, because
// there is nothing outside it worth reaching.
func (h Host) resolve(name string) (string, error) {
	if name == "" {
		return "", errors.New("path is empty")
	}
	root, err := filepath.Abs(h.Dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, name)
	if filepath.IsAbs(name) {
		path = filepath.Clean(name)
	}
	if path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the working directory", name)
	}
	return path, nil
}
