// Package editfiletool provides a tool to make targeted edits to a file.
package editfiletool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	editName        = "edit_file"
	editDescription = "Makes targeted edits to a file by replacing specific text. Use this for precise modifications without rewriting the entire file."
)

// New creates a new file edit tool.
func New() *editFileTool {
	return &editFileTool{}
}

type editFileTool struct{}

func (t *editFileTool) Name() string        { return editName }
func (t *editFileTool) Description() string { return editDescription }

func (t *editFileTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: path")
	}

	oldStr, ok := args["old_string"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: old_string")
	}

	newStr, ok := args["new_string"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: new_string")
	}

	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil, fmt.Errorf("path must be absolute: %s", path)
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	original := string(content)
	if !strings.Contains(original, oldStr) {
		return nil, nil, fmt.Errorf("old_string not found in file. Make sure the text matches exactly including whitespace.")
	}

	// Check for multiple occurrences
	count := strings.Count(original, oldStr)
	if count > 1 {
		return nil, nil, fmt.Errorf("old_string appears %d times. Make it more specific to match only once.", count)
	}

	newContent := strings.Replace(original, oldStr, newStr, 1)

	if err := os.WriteFile(cleanPath, []byte(newContent), 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	return map[string]any{
		"success": true,
		"path":    cleanPath,
	}, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *editFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The absolute path of the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The exact text to find and replace. Must match exactly including whitespace.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The replacement text.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}
