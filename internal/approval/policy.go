package approval

import (
	"encoding/json"
	"slices"
	"strings"
)

// Policy says which calls run without a question. Everything it does not name
// waits for a person, so a new tool is gated until someone decides otherwise.
type Policy struct {
	// Free are the tools that never need a question.
	Free []string
	// Commands are the programs the shell tool may run without one.
	Commands []string
}

// Default reads and builds without asking, and asks before anything else.
// Reading and writing files is free because they stay inside the workspace;
// fetch is missing from Free on purpose, because it reaches the network.
func Default() Policy {
	return Policy{
		Free: []string{"read_file", "write_file"},
		Commands: []string{
			"cat", "cd", "diff", "echo", "false", "file", "find", "grep", "head",
			"ls", "mkdir", "printf", "pwd", "sed", "sort", "stat", "tail", "test",
			"true", "uniq", "wc", "which",
			"cargo", "git", "go", "gofmt", "make", "node", "npm", "python3",
		},
	}
}

// Allows reports whether one call may run with nobody watching.
func (p Policy) Allows(tool string, args json.RawMessage) bool {
	if slices.Contains(p.Free, tool) {
		return true
	}
	if tool != "shell" {
		return false
	}

	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return false
	}
	return p.allowsCommand(in.Command)
}

// allowsCommand walks every part of a command line, because one allowed
// program at the front says nothing about what follows the semicolon.
func (p Policy) allowsCommand(command string) bool {
	// A substitution can hide any program inside an allowed one.
	if strings.Contains(command, "$(") || strings.Contains(command, "`") {
		return false
	}

	parts := strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '|' || r == '&' || r == '\n'
	})
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !slices.Contains(p.Commands, program(part)) {
			return false
		}
	}
	return true
}

// program is the name a part of a command line runs. A leading environment
// assignment hides it, so such a part is never free.
func program(part string) string {
	fields := strings.Fields(part)
	if len(fields) == 0 || strings.Contains(fields[0], "=") {
		return ""
	}
	return fields[0]
}
