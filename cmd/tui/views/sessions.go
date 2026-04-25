package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
)

type PickerItem struct {
	Title   string
	Detail  string
	Preview string
}

func RenderSessions(theme render.Theme, width int, items []PickerItem, selected int) string {
	return renderPicker(theme, width, "Sessions", "", items, selected)
}

func renderPicker(theme render.Theme, width int, title, hint string, items []PickerItem, selected int) string {
	if width <= 0 {
		width = 88
	}
	rows := []string{theme.Assistant.Render(strings.ToLower(title)), ""}
	if hint != "" {
		rows = append(rows, theme.Hint.Render(hint), "")
	}
	if len(items) == 0 {
		rows = append(rows, theme.Subtle.Render("No items"))
	} else {
		start, end := pickerWindow(len(items), selected, 8)
		if start > 0 {
			rows = append(rows, theme.Subtle.Render(fmt.Sprintf("… %d earlier", start)), "")
		}
		for i := start; i < end; i++ {
			item := items[i]
			titleLine := "  " + trimToWidth(item.Title, width-8)
			detailLine := "    " + trimToWidth(item.Detail, width-8)
			previewLine := "    " + theme.Subtle.Render(trimToWidth(item.Preview, width-8))
			if i == selected {
				titleLine = theme.Selection.Width(width - 4).Render(titleLine)
				detailLine = theme.Selection.Width(width - 4).Render(detailLine)
				previewLine = theme.Selection.Width(width - 4).Render(previewLine)
			}
			rows = append(rows, titleLine, detailLine, previewLine)
		}
		if end < len(items) {
			rows = append(rows, "", theme.Subtle.Render(fmt.Sprintf("… %d more", len(items)-end)))
		}
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return theme.Modal.Width(width).Render(content)
}

func pickerWindow(total, selected, maxVisible int) (int, int) {
	if total <= maxVisible {
		return 0, total
	}
	start := selected - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
	}
	return start, end
}
