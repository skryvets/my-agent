package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type readArgs struct {
	Path string `json:"path" jsonschema:"required,description=The file to read. The path is relative to the working directory"`
}

// ReadFile returns the text of one file in the workspace.
func ReadFile(workspace Workspace) (tool.BaseTool, error) {
	return utils.InferTool("read_file",
		"Read a text file and return its content. The path is relative to the working directory.",
		func(ctx context.Context, in readArgs) (string, error) {
			content, err := workspace.ReadFile(ctx, in.Path)
			if err != nil {
				return "", err
			}
			return truncate(content), nil
		})
}

type writeArgs struct {
	Path    string `json:"path" jsonschema:"required,description=The file to write. The path is relative to the working directory"`
	Content string `json:"content" jsonschema:"required,description=The whole new content of the file"`
}

// WriteFile replaces the content of one file in the workspace.
func WriteFile(workspace Workspace) (tool.BaseTool, error) {
	return utils.InferTool("write_file",
		"Write a text file, replacing it if it exists. "+
			"Missing parent directories are created. The path is relative to the working directory.",
		func(ctx context.Context, in writeArgs) (string, error) {
			if err := workspace.WriteFile(ctx, in.Path, in.Content); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
		})
}
