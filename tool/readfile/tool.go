// Package readfiletool provides a tool to read file contents.
package readfiletool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	readName        = "read_file"
	readDescription = "Reads the contents of a file at the specified path. Returns the file content as a string."
)

// New creates a new file read tool.
func New() *readFileTool {
	return &readFileTool{}
}

type readFileTool struct{}

func (t *readFileTool) Name() string        { return readName }
func (t *readFileTool) Description() string { return readDescription }

func (t *readFileTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: path")
	}

	// Clean and validate path
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil, fmt.Errorf("path must be absolute: %s", path)
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("path is a directory, not a file: %s", path)
	}

	const maxFileSize = 100000
	if info.Size() > maxFileSize {
		return nil, nil, fmt.Errorf("file too large (%d bytes, max %d): %s", info.Size(), maxFileSize, path)
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	return map[string]any{
		"content": string(content),
		"path":    cleanPath,
		"size":    info.Size(),
	}, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *readFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The absolute path of the file to read.",
			},
		},
		"required": []string{"path"},
	}
}
