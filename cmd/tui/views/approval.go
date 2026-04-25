package views

import (
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
)

type ApprovalData struct {
	Tool    string
	Message string
	Args    string
	Pending int
}

func RenderApproval(theme render.Theme, width int, data ApprovalData) string {
	if width <= 0 {
		width = 80
	}

	lines := []string{
		theme.Tool.Render("approval"),
		"",
		theme.Message.Render(fmt.Sprintf("tool  %s", data.Tool)),
		theme.Subtle.Render(data.Message),
	}
	if strings.TrimSpace(data.Args) != "" {
		lines = append(lines, "", theme.Subtle.Render("arguments"), theme.Panel.Width(width-4).Render(render.Markdown(data.Args, width-6)))
	}
	lines = append(lines, "", theme.Subtle.Render(fmt.Sprintf("%d pending    1 approve    2 reject    3 later", max(data.Pending, 1))))

	content := strings.Join(lines, "\n")
	return theme.Modal.Width(width).Render(content)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
