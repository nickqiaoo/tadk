package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
)

type SlashSuggestion struct {
	Name        string
	Description string
}

type ComposerState struct {
	Input        string
	Status       string
	Suggestions  []SlashSuggestion
	SelectedIdx  int
	ShowSuggestions bool
}

func RenderComposer(theme render.Theme, width int, state ComposerState) string {
	if width <= 0 {
		width = 80
	}

	status := strings.TrimSpace(state.Status)
	if status == "" {
		status = "Ready"
	}

	var parts []string

	// Suggestions overlay above input
	if state.ShowSuggestions && len(state.Suggestions) > 0 {
		parts = append(parts, renderSuggestions(theme, width, state.Suggestions, state.SelectedIdx))
	}

	meta := theme.Status.Width(width - 2).Render(trimToWidth(status, width-2))
	body := theme.Composer.Width(width).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		meta,
		state.Input,
	))

	parts = append(parts,
		theme.Divider.Render(strings.Repeat("─", width)),
		body,
	)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		parts...,
	)
}

func renderSuggestions(theme render.Theme, width int, suggestions []SlashSuggestion, selectedIdx int) string {
	if width <= 0 {
		width = 80
	}
	maxVisible := 6
	if len(suggestions) < maxVisible {
		maxVisible = len(suggestions)
	}

	var lines []string
	for i := 0; i < maxVisible; i++ {
		s := suggestions[i]
		nameWidth := 16
		name := s.Name
		if len([]rune(name)) > nameWidth {
			name = string([]rune(name)[:nameWidth-1]) + "…"
		}
		padding := strings.Repeat(" ", maxInt(0, nameWidth-len([]rune(name))))
		desc := s.Description
		descMax := width - nameWidth - 6
		if descMax > 0 && len([]rune(desc)) > descMax {
			desc = string([]rune(desc)[:descMax-1]) + "…"
		}

		if i == selectedIdx {
			line := theme.SuggestionSelected.Render(" /"+name+padding) + "  " + theme.Message.Render(desc)
			lines = append(lines, line)
		} else {
			line := theme.SuggestionDim.Render(" /"+name+padding) + "  " + theme.Subtle.Render(desc)
			lines = append(lines, line)
		}
	}
	if len(suggestions) > maxVisible {
		lines = append(lines, theme.Subtle.Render("  … and "+strconvItoa(len(suggestions)-maxVisible)+" more"))
	}
	return strings.Join(lines, "\n")
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf) - 1
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}
