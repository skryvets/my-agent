// Package tools is the set of actions the agent can take in a dev container: a
// shell command, a file read and a file write. Each one is an eino
// tool.InvokableTool whose arguments schema is read from a Go struct.
package tools

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
)

// outputLimit caps what one tool returns. A build log can be megabytes, and the
// whole result goes back into the history on every later turn.
const outputLimit = 8000

// truncate keeps the tail of long output, because the end of a build log says
// what went wrong.
func truncate(text string) string {
	if len(text) <= outputLimit {
		return text
	}
	cut := len(text) - outputLimit
	return fmt.Sprintf("[%d bytes cut, the last %d follow]\n%s", cut, outputLimit, text[cut:])
}

// All is every tool, bound to one workspace. Adding a tool is a new file in
// this package and a name in this list.
func All(workspace Workspace) ([]tool.BaseTool, error) {
	var kit []tool.BaseTool
	for _, build := range []func(Workspace) (tool.BaseTool, error){Shell, ReadFile, WriteFile} {
		built, err := build(workspace)
		if err != nil {
			return nil, err
		}
		kit = append(kit, built)
	}
	return kit, nil
}
