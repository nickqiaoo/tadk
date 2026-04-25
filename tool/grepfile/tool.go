// Package grepfiletool provides a tool to search file contents by pattern.
package grepfiletool

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	grepName        = "grep_search"
	grepDescription = "Searches file contents using regex patterns. Returns matching lines with file paths and line numbers."
)

// New creates a new grep search tool.
func New() *grepFileTool {
	return &grepFileTool{}
}

type grepFileTool struct{}

func (t *grepFileTool) Name() string        { return grepName }
func (t *grepFileTool) Description() string { return grepDescription }

func (t *grepFileTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: path")
	}

	pattern, ok := args["pattern"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: pattern")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil, fmt.Errorf("path must be absolute: %s", path)
	}

	filePattern := "*"
	if fp, ok := args["file_pattern"].(string); ok {
		filePattern = fp
	}

	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	type match struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}

	var results []match

	err = filepath.WalkDir(cleanPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".venv" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		matched, _ := filepath.Match(filePattern, d.Name())
		if !matched {
			return nil
		}

		file, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				results = append(results, match{
					File:    p,
					Line:    lineNum,
					Content: strings.TrimSpace(line),
				})
				if len(results) >= maxResults {
					return filepath.SkipDir
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search: %w", err)
	}

	return map[string]any{
		"matches":   results,
		"count":     len(results),
		"truncated": len(results) >= maxResults,
	}, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *grepFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The absolute directory path to search in.",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "The regex pattern to search for in file contents.",
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. '*.go'). Defaults to '*'.",
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": "Maximum number of matches to return. Defaults to 50.",
			},
		},
		"required": []string{"path", "pattern"},
	}
}
