package tools

import (
	"context"
	"errors"
)

// fakeWorkspace records what a tool asked for and answers with what the test
// set, so a tool can be tested without a host or a container.
type fakeWorkspace struct {
	command string
	path    string
	content string

	output string
	err    error
}

func (f *fakeWorkspace) Run(ctx context.Context, command string) (string, error) {
	f.command = command
	return f.output, f.err
}

func (f *fakeWorkspace) ReadFile(ctx context.Context, name string) (string, error) {
	f.path = name
	return f.output, f.err
}

func (f *fakeWorkspace) WriteFile(ctx context.Context, name, content string) error {
	f.path, f.content = name, content
	return f.err
}

var errWorkspace = errors.New("the workspace is gone")
