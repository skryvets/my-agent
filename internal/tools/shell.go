package tools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// shellTimeout stops a command that never ends.
const shellTimeout = 2 * time.Minute

type shellArgs struct {
	Command string `json:"command" jsonschema:"required,description=The command line to run. An example: go test ./... -race"`
}

// Shell runs a command line in the workspace.
func Shell(workspace Workspace) (tool.BaseTool, error) {
	return utils.InferTool("shell",
		"Run a shell command with sh -c and return its combined output. "+
			"Use it to list files, to search, to build and to run tests.",
		func(ctx context.Context, in shellArgs) (string, error) {
			return runShell(ctx, workspace, in.Command)
		})
}

func runShell(ctx context.Context, workspace Workspace, command string) (string, error) {
	if command == "" {
		return "", errors.New("command is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()

	output, err := workspace.Run(ctx, command)
	report := truncate(output)
	if ctx.Err() != nil {
		return report + fmt.Sprintf("\n[stopped after %s]", shellTimeout), nil
	}
	if err != nil {
		return "", err
	}
	if report == "" {
		return "[no output, exit 0]", nil
	}
	return report, nil
}
