package render

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	App        lipgloss.Style
	Header     lipgloss.Style
	HeaderMeta lipgloss.Style
	HeaderKey  lipgloss.Style
	Status     lipgloss.Style
	Hint       lipgloss.Style
	User       lipgloss.Style
	Assistant  lipgloss.Style
	Thinking   lipgloss.Style
	Tool       lipgloss.Style
	ToolBadge  lipgloss.Style
	ToolSuccess lipgloss.Style
	ToolError  lipgloss.Style
	System     lipgloss.Style
	Error      lipgloss.Style
	Modal      lipgloss.Style
	Selection  lipgloss.Style
	Subtle     lipgloss.Style
	Transcript lipgloss.Style
	Message    lipgloss.Style
	Panel      lipgloss.Style
	Composer   lipgloss.Style
	Divider    lipgloss.Style
	Dot        lipgloss.Style
	MutedBorder lipgloss.Style
	SuggestionSelected lipgloss.Style
	SuggestionDim      lipgloss.Style
}

func NewTheme() Theme {
	const (
		bg           = "#0d0d0d"
		surface      = "#1a1a1a"
		surfaceHi    = "#222222"
		border       = "#333333"
		text         = "#e0e0e0"
		muted        = "#666666"
		accent       = "#b1b9f9" // light blue-purple (Claude Code permission/suggestion)
		userGreen    = "#7ee787"
		claudeOrange = "#d78777"
		toolYellow   = "#e3b341"
		successGreen = "#4eba65"
		errorRed     = "#ff6b80"
	)

	return Theme{
		App:        lipgloss.NewStyle().Foreground(lipgloss.Color(text)),
		Header:     lipgloss.NewStyle().Foreground(lipgloss.Color(text)).Padding(0, 1),
		HeaderMeta: lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		HeaderKey:  lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true),
		Status:     lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		Hint:       lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		User:       lipgloss.NewStyle().Foreground(lipgloss.Color(userGreen)),
		Assistant:  lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		Thinking:   lipgloss.NewStyle().Foreground(lipgloss.Color(muted)).Italic(true),
		Tool:       lipgloss.NewStyle().Foreground(lipgloss.Color(toolYellow)),
		ToolBadge:  lipgloss.NewStyle().Foreground(lipgloss.Color(bg)).Background(lipgloss.Color(toolYellow)).Bold(true).Padding(0, 1),
		ToolSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color(successGreen)),
		ToolError:  lipgloss.NewStyle().Foreground(lipgloss.Color(errorRed)),
		System:     lipgloss.NewStyle().Foreground(lipgloss.Color(userGreen)),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color(errorRed)),
		Modal:      lipgloss.NewStyle().Padding(1, 2),
		Selection:  lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true),
		Subtle:     lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
		Transcript: lipgloss.NewStyle().Padding(0, 1),
		Message:    lipgloss.NewStyle().Foreground(lipgloss.Color(text)),
		Panel:      lipgloss.NewStyle().Padding(0, 1),
		Composer:   lipgloss.NewStyle().Padding(0, 1),
		Divider:    lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		Dot:        lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true),
		MutedBorder: lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		SuggestionSelected: lipgloss.NewStyle().Background(lipgloss.Color("#1e2a3a")).Foreground(lipgloss.Color(accent)),
		SuggestionDim:      lipgloss.NewStyle().Foreground(lipgloss.Color(muted)),
	}
}
