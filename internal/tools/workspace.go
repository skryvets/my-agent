package tools

import "context"

// Workspace is where the tools work: the dev container of one task. A tool
// reaches it only through this interface.
type Workspace interface {
	// Run executes one command line with sh -c and returns the combined
	// output. A command that exits non-zero is reported in the text, not as
	// an error, because the model reads it and decides what to do.
	Run(ctx context.Context, command string) (string, error)
	ReadFile(ctx context.Context, name string) (string, error)
	WriteFile(ctx context.Context, name, content string) error
}
