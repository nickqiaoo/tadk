package views

import (
	"strings"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
)

const (
	blackCircle = "●"
	dotPrefix   = "▸ "
)

func RenderChat(theme render.Theme, width int, content string) string {
	content = strings.TrimRight(content, "\n")
	if width > 0 {
		theme.Transcript = theme.Transcript.Width(width)
	}
	return theme.Transcript.Render(content)
}

func RenderBlock(theme render.Theme, kind, label, body string, width int) string {
	if width <= 0 {
		width = 80
	}

	bodyWidth := maxInt(20, width-4)

	switch kind {
	case "user":
		body = render.Markdown(body, bodyWidth)
		return theme.User.Render(dotPrefix) + theme.Message.Render(body)
	case "assistant":
		body = render.Markdown(body, bodyWidth)
		dot := theme.Dot.Render(blackCircle + " ")
		return dot + theme.Message.Render(body)
	case "thinking":
		body = render.Markdown(body, bodyWidth)
		prefix := theme.Thinking.Render("... ")
		return prefix + theme.Thinking.Render(body)
	case "tool":
		return renderToolBlock(theme, label, body, width)
	case "error":
		body = render.Markdown(body, bodyWidth)
		return theme.Error.Render("✗ ") + theme.Error.Render(body)
	case "system":
		body = render.Markdown(body, bodyWidth)
		return theme.System.Render("ℹ ") + theme.Subtle.Render(body)
	case "banner":
		return body
	}

	body = render.Markdown(body, bodyWidth)
	return theme.Message.Render(body)
}

func renderToolBlock(theme render.Theme, label, body string, width int) string {
	// label is something like "Tool Call: bash" or "Tool: bash"
	var toolName string
	isResult := false
	if trimmed, ok := strings.CutPrefix(label, "Tool Call: "); ok {
		toolName = trimmed
	} else if trimmed, ok := strings.CutPrefix(label, "Tool: "); ok {
		toolName = trimmed
		isResult = true
	} else {
		toolName = label
	}

	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	// First line: badge + args summary
	var firstLine string
	badge := theme.ToolBadge.Render(" "+toolName+" ")

	isError := false
	if isResult {
		first := lines[0]
		if strings.Contains(first, "error") || strings.HasPrefix(first, "exit ") {
			if !strings.HasPrefix(first, "exit 0") {
				isError = true
			}
		}
		statusIcon := theme.ToolSuccess.Render("✓")
		if isError {
			statusIcon = theme.ToolError.Render("✗")
		}
		firstLine = statusIcon + " " + badge + "  " + theme.Subtle.Render(lines[0])
	} else {
		firstLine = theme.Dot.Render(blackCircle+" ") + badge + "  " + theme.Subtle.Render(lines[0])
	}

	// Remaining lines (output preview) indented
	var rest []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rest = append(rest, theme.Subtle.Render("  "+line))
	}

	out := []string{firstLine}
	out = append(out, rest...)
	return strings.Join(out, "\n")
}

