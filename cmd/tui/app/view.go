package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
	"github.com/nickqiaoo/tadk/cmd/tui/views"
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	header := views.RenderHeader(m.theme, m.width, currentSessionID(m.currentSession), m.currentModel.Label(), m.stateLabel())
	chat := views.RenderChat(m.theme, m.width, m.viewport.View())

	// Build slash suggestions for composer
	var slashSugs []views.SlashSuggestion
	for _, s := range m.slashSuggestions {
		slashSugs = append(slashSugs, views.SlashSuggestion{Name: s.name, Description: s.description})
	}
	composer := views.RenderComposer(m.theme, m.width, views.ComposerState{
		Input:        m.input.View(),
		Status:       m.status,
		Suggestions:  slashSugs,
		SelectedIdx:  m.slashSelectedIdx,
		ShowSuggestions: m.slashSuggestionsActive,
	})

	parts := []string{header, chat}
	if modal := m.renderModal(); modal != "" {
		parts = append(parts, views.RenderOverlayDivider(m.theme, m.width), modal)
	}
	parts = append(parts, composer)

	return m.theme.App.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m *Model) renderTranscript() string {
	width := m.viewport.Width - 4
	if width < 20 {
		width = max(20, m.viewport.Width)
	}

	var blocks []string
	for _, block := range m.transcript {
		blocks = append(blocks, views.RenderBlock(m.theme, block.kind, block.title, block.body, width))
	}
	return strings.Join(blocks, "\n\n")
}

func (m *Model) renderModal() string {
	switch m.modal {
	case modalApproval:
		if len(m.approvalQueue) == 0 {
			return ""
		}
		item := m.approvalQueue[0]
		return views.RenderApproval(m.theme, m.width, views.ApprovalData{
			Tool:    item.Tool,
			Message: item.Message,
			Args:    render.FormatValue(item.Args),
			Pending: len(m.approvalQueue),
		})
	case modalSessions:
		items := make([]views.PickerItem, 0, len(m.sessionItems))
		for _, item := range m.sessionItems {
			items = append(items, views.PickerItem{
				Title:   item.ID,
				Detail:  item.Detail(),
				Preview: item.Preview,
			})
		}
		return views.RenderSessions(m.theme, m.width, items, m.sessionIndex)
	case modalModels:
		items := make([]views.PickerItem, 0, len(m.modelItems))
		for _, item := range m.modelItems {
			items = append(items, views.PickerItem{
				Title:  item.Provider,
				Detail: item.Name,
			})
		}
		return views.RenderModelPicker(m.theme, m.width, items, m.modelIndex)
	default:
		return ""
	}
}

func (m *Model) stateLabel() string {
	switch {
	case m.runActive:
		return "running"
	case len(m.approvalQueue) > 0:
		return "approval"
	default:
		return "idle"
	}
}

func currentSessionID(sess interface{ ID() string }) string {
	if sess == nil {
		return "-"
	}
	return sess.ID()
}

func formatStatus(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
