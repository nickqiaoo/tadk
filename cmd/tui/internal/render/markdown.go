package render

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Markdown renders markdown text to ANSI-colored terminal output.
// If width is <= 0, it returns the raw text trimmed.
func Markdown(text string, width int) string {
	text = strings.TrimRight(text, "\n")
	if width <= 0 {
		return text
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
		glamour.WithStylesFromJSONBytes([]byte(`{"document":{"block_prefix":"","margin":0}}`)),
	)
	if err != nil {
		// fallback to plain text with width wrapping
		return lipgloss.NewStyle().Width(width).Render(text)
	}

	out, err := r.Render(text)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(text)
	}

	out = strings.TrimRight(out, "\n")
	return out
}
