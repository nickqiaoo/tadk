package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Banner returns a colorful ASCII-art "TADK" string.
func Banner() string {
	lines := []string{
		"████████╗  █████╗  ██████╗  ██╗  ██╗",
		"╚══██╔══╝ ██╔══██╗ ██╔══██╗ ██║ ██╔╝",
		"   ██║    ███████║ ██║  ██║ █████╔╝ ",
		"   ██║    ██╔══██║ ██║  ██║ ██╔═██╗ ",
		"   ██║    ██║  ██║ ██████╔╝ ██║  ██╗",
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#b1b9f9"))
	var out []string
	for _, line := range lines {
		out = append(out, style.Render(line))
	}
	return strings.Join(out, "\n")
}
