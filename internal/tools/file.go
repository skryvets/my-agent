package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ReadFile returns the text of one file in the workspace.
type ReadFile struct {
	Workspace Workspace
}

func (r ReadFile) Name() string { return "read_file" }

func (r ReadFile) Description() string {
	return "Read a text file and return its content. The path is relative to the working directory."
}

func (r ReadFile) Parameters() map[string]any { return pathSchema("The file to read", false) }

func (r ReadFile) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var in fileArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	content, err := r.Workspace.ReadFile(ctx, in.Path)
	if err != nil {
		return "", err
	}
	return truncate(content), nil
}

// WriteFile replaces the content of one file in the workspace.
type WriteFile struct {
	Workspace Workspace
}

func (w WriteFile) Name() string { return "write_file" }

func (w WriteFile) Description() string {
	return "Write a text file, replacing it if it exists. " +
		"Missing parent directories are created. The path is relative to the working directory."
}

func (w WriteFile) Parameters() map[string]any { return pathSchema("The file to write", true) }

func (w WriteFile) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var in fileArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	if err := w.Workspace.WriteFile(ctx, in.Path, in.Content); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

type fileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func pathSchema(pathDescription string, withContent bool) map[string]any {
	properties := map[string]any{
		"path": map[string]any{"type": "string", "description": pathDescription},
	}
	required := []string{"path"}
	if withContent {
		properties["content"] = map[string]any{
			"type":        "string",
			"description": "The whole new content of the file",
		}
		required = append(required, "content")
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}
