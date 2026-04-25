package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
)

func RenderHeader(theme render.Theme, width int, sessionID, modelLabel, state string) string {
	if width <= 0 {
		width = 80
	}
	left := lipgloss.JoinHorizontal(
		lipgloss.Left,
		theme.HeaderKey.Render("tadk"),
		theme.HeaderMeta.Render(" · "+sessionID),
	)
	right := lipgloss.JoinHorizontal(
		lipgloss.Left,
		theme.HeaderMeta.Render(state+" · "),
		theme.Assistant.Render(modelLabel),
	)
	spacerW := width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacerW < 1 {
		spacerW = 1
	}
	line := lipgloss.JoinHorizontal(lipgloss.Left, left, strings.Repeat(" ", spacerW), right)
	return theme.Header.Width(width).Render(line)
}

func RenderFooter(theme render.Theme, width int, status string) string {
	if width <= 0 {
		width = 80
	}
	return theme.Status.Width(width).Render(trimToWidth(status, width))
}

func RenderOverlayDivider(theme render.Theme, width int) string {
	if width <= 0 {
		width = 80
	}
	return theme.Divider.Render(strings.Repeat("▔", width))
}

