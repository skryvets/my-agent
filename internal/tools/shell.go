package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// shellTimeout stops a command that never ends.
const shellTimeout = 2 * time.Minute

// Shell runs a command with sh -c.
type Shell struct {
	// Dir is the working directory. An empty Dir means the current one.
	Dir string
}

func (s Shell) Name() string { return "shell" }

func (s Shell) Description() string {
	return "Run a shell command with sh -c and return its combined output. " +
		"Use it to list files, to search, to build and to run tests."
}

func (s Shell) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command line to run, for example: go test ./... -race",
			},
		},
		"required": []string{"command"},
	}
}

func (s Shell) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Command string `json:"command"`
	}
	if err := decode(args, &in); err != nil {
		return "", err
	}
	if in.Command == "" {
		return "", errors.New("command is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
	cmd.Dir = s.Dir
	output, err := cmd.CombinedOutput()

	// A command that fails is an answer, not a failure of the tool. The model
	// reads the exit status and the output and decides what to do next.
	report := truncate(string(output))
	if ctx.Err() != nil {
		return report + fmt.Sprintf("\n[stopped after %s]", shellTimeout), nil
	}
	if err != nil {
		return report + fmt.Sprintf("\n[exit: %v]", err), nil
	}
	if report == "" {
		return "[no output, exit 0]", nil
	}
	return report, nil
}
