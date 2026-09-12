package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile returns the text of one file under Dir.
type ReadFile struct {
	// Dir is the root the paths are resolved against. An empty Dir means the
	// current directory.
	Dir string
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
	path, err := resolve(r.Dir, in.Path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return truncate(string(content)), nil
}

// WriteFile replaces the content of one file under Dir.
type WriteFile struct {
	// Dir is the root the paths are resolved against. An empty Dir means the
	// current directory.
	Dir string
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
	path, err := resolve(w.Dir, in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
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

// resolve keeps a path inside the root, so a model that asks for ../../etc
// gets an error instead of the file.
func resolve(dir, name string) (string, error) {
	if name == "" {
		return "", errors.New("path is empty")
	}
	root, err := filepath.Abs(dir)
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
