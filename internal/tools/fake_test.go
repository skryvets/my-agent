package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
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

// build makes one tool, or ends the test when the arguments schema is wrong.
func build(t *testing.T, make func(Workspace) (tool.BaseTool, error), workspace Workspace) tool.InvokableTool {
	t.Helper()
	built, err := make(workspace)
	if err != nil {
		t.Fatalf("build the tool: %v", err)
	}
	invokable, ok := built.(tool.InvokableTool)
	if !ok {
		t.Fatalf("the tool cannot be invoked: %T", built)
	}
	return invokable
}

// call runs one tool with the arguments the model would send.
func call(t *testing.T, invokable tool.InvokableTool, arguments string) (string, error) {
	t.Helper()
	return invokable.InvokableRun(context.Background(), arguments)
}

// properties reads the JSON Schema of the arguments of one tool.
func properties(t *testing.T, invokable tool.InvokableTool) map[string]bool {
	t.Helper()
	info, err := invokable.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	arguments, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	fields := map[string]bool{}
	for pair := arguments.Properties.Oldest(); pair != nil; pair = pair.Next() {
		fields[pair.Key] = true
	}
	return fields
}
