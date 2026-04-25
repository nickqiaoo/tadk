// Package writefiletool provides a tool to write content to a file.
package writefiletool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	writeName        = "write_file"
	writeDescription = "Writes content to a file at the specified path. Creates the file if it doesn't exist, overwrites if it does. Creates parent directories if needed."
)

// New creates a new file write tool.
func New() *writeFileTool {
	return &writeFileTool{}
}

type writeFileTool struct{}

func (t *writeFileTool) Name() string        { return writeName }
func (t *writeFileTool) Description() string { return writeDescription }

func (t *writeFileTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: path")
	}

	content, ok := args["content"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: content")
	}

	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil, fmt.Errorf("path must be absolute: %s", path)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	return map[string]any{
		"success": true,
		"path":    cleanPath,
	}, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *writeFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The absolute path of the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}
