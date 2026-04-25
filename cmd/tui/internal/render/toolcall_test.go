package render

import (
	"strings"
	"testing"
)

func TestFormatToolCallBash(t *testing.T) {
	got := FormatToolCall("bash", map[string]any{
		"command": "rg --files\nsed -n '1,20p' go.mod",
	})
	if !strings.Contains(got, "$ rg --files") {
		t.Fatalf("expected bash summary, got %q", got)
	}
	if !strings.Contains(got, "sed -n") {
		t.Fatalf("expected multiline command preview, got %q", got)
	}
	if strings.Contains(got, "\"command\"") {
		t.Fatalf("expected no raw json, got %q", got)
	}
}

func TestFormatToolCallReadFile(t *testing.T) {
	got := FormatToolCall("read_file", map[string]any{
		"path":   "/Volumes/data/project/tadk/cmd/tui/app/update.go",
		"offset": 10,
		"limit":  20,
	})
	if !strings.Contains(got, "update.go") || !strings.Contains(got, "lines 10-29") {
		t.Fatalf("unexpected read_file summary: %q", got)
	}
}

func TestFormatToolResultBash(t *testing.T) {
	got := FormatToolResult("bash", map[string]any{
		"exit_code": 0,
		"output":    "alpha\nbeta\ngamma\n",
	}, false)
	if !strings.Contains(got, "exit 0") || !strings.Contains(got, "alpha") {
		t.Fatalf("unexpected bash result: %q", got)
	}
	if strings.Contains(got, "\"output\"") {
		t.Fatalf("expected no raw json, got %q", got)
	}
}

func TestFormatToolResultGrep(t *testing.T) {
	got := FormatToolResult("grep_search", map[string]any{
		"count": 2,
		"matches": []map[string]any{
			{"file": "/Volumes/data/project/tadk/go.mod", "line": 3, "content": "require ("},
			{"file": "/Volumes/data/project/tadk/go.mod", "line": 9, "content": "github.com/charmbracelet/bubbletea"},
		},
	}, false)
	if !strings.Contains(got, "2 matches") || !strings.Contains(got, "go.mod:3") {
		t.Fatalf("unexpected grep result: %q", got)
	}
}
