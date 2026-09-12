// Package tools is the set of actions the agent can take: a shell command, a
// file read, a file write and an HTTP fetch. Each type satisfies agent.Tool.
package tools

import (
	"encoding/json"
	"fmt"
)

// outputLimit caps what one tool returns. A build log can be megabytes, and the
// whole result goes back into the history on every later turn.
const outputLimit = 8000

// decode reads the arguments object the model produced.
func decode(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("cannot read the arguments: %v", err)
	}
	return nil
}

// truncate keeps the tail of long output, because the end of a build log says
// what went wrong.
func truncate(text string) string {
	if len(text) <= outputLimit {
		return text
	}
	cut := len(text) - outputLimit
	return fmt.Sprintf("[%d bytes cut, the last %d follow]\n%s", cut, outputLimit, text[cut:])
}
