// Package findfiletool provides a tool to search for files by name or pattern.
package findfiletool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	findName        = "find_file"
	findDescription = "Searches for files by name pattern in a directory. Supports glob patterns like *.go or **/*.md."
)

// New creates a new file find tool.
func New() *findFileTool {
	return &findFileTool{}
}

type findFileTool struct{}

func (t *findFileTool) Name() string        { return findName }
func (t *findFileTool) Description() string { return findDescription }

func (t *findFileTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: path")
	}

	pattern := "**"
	if p, ok := args["pattern"].(string); ok {
		pattern = p
	}

	maxResults := 100
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil, fmt.Errorf("path must be absolute: %s", path)
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("path is not a directory: %s", path)
	}

	var results []string
	err = filepath.WalkDir(cleanPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors, continue walking
		}

		// Skip hidden directories and common non-source dirs
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".venv" || name == "vendor" {
				return filepath.SkipDir
			}
		}

		matched, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return nil
		}
		if matched {
			results = append(results, p)
			if len(results) >= maxResults {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search: %w", err)
	}

	return map[string]any{
		"files":     results,
		"count":     len(results),
		"truncated": len(results) >= maxResults,
	}, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *findFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The absolute directory path to search in.",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match filenames (e.g. '*.go', '*.md'). Defaults to '**'.",
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": "Maximum number of results to return. Defaults to 100.",
			},
		},
		"required": []string{"path"},
	}
}
