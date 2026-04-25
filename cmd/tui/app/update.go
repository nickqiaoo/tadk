package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/cmd/tui/bridge"
	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
	"github.com/nickqiaoo/tadk/cmd/tui/runtime"
	"github.com/nickqiaoo/tadk/cmd/tui/storage"
	"github.com/nickqiaoo/tadk/cmd/tui/views"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/resume"
	interrupttool "github.com/nickqiaoo/tadk/tool/interrupt"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applySize()
		return m, nil

	case tea.KeyMsg:
		if handled, cmd := m.handleGlobalKey(msg); handled {
			return m, cmd
		}
		return m.handleKey(msg)

	case bridge.ErrorMsg:
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
			m.status = msg.Err.Error()
		}
		return m, nil

	case bridge.EventMsg:
		m.handleEvent(msg.Event)
		return m, nil
	}

	var cmds []tea.Cmd
	if m.modal == modalNone {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)
	}
	var viewportCmd tea.Cmd
	m.viewport, viewportCmd = m.viewport.Update(msg)
	cmds = append(cmds, viewportCmd)
	return m, tea.Batch(cmds...)
}

func (m *Model) handleGlobalKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.Type != tea.KeyCtrlC && m.exitRequested {
		m.exitRequested = false
	}

	if msg.Type == tea.KeyEsc && m.modal == modalNone && m.runActive && m.runCancel != nil {
		m.status = "Interrupting current run..."
		m.runCancel()
		return true, nil
	}

	if msg.Type != tea.KeyCtrlC {
		return false, nil
	}

	if !m.exitRequested {
		m.exitRequested = true
		m.status = "Press Ctrl+C again to exit."
		return true, nil
	}
	m.cancel()
	return true, tea.Quit
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m.handleModalKey(msg)
	}

	switch msg.String() {
	case "pgup":
		m.viewport.PageUp()
		return m, nil
	case "pgdown":
		m.viewport.PageDown()
		return m, nil
	case "ctrl+u":
		m.viewport.HalfPageUp()
		return m, nil
	case "ctrl+d":
		m.viewport.HalfPageDown()
		return m, nil
	case "up", "k":
		if m.slashSuggestionsActive {
			if m.slashSelectedIdx > 0 {
				m.slashSelectedIdx--
			}
			return m, nil
		}
	case "down", "j":
		if m.slashSuggestionsActive {
			if m.slashSelectedIdx < len(m.slashSuggestions)-1 {
				m.slashSelectedIdx++
			}
			return m, nil
		}
	case "tab":
		if m.slashSuggestionsActive {
			m.acceptSlashSuggestion()
			return m, nil
		}
	case "esc":
		if m.slashSuggestionsActive {
			m.slashSuggestionsActive = false
			m.slashSelectedIdx = 0
			return m, nil
		}
	case "enter":
		if m.slashSuggestionsActive && len(m.slashSuggestions) > 0 && m.slashSelectedIdx < len(m.slashSuggestions) {
			selected := m.slashSuggestions[m.slashSelectedIdx]
			m.input.Reset()
			m.slashSuggestionsActive = false
			m.slashSelectedIdx = 0
			m.refreshInputHeight()
			return m, m.handleSlash(slashCommand{name: selected.name})
		}
		text := strings.TrimSpace(m.input.Value())
		if text == "" || m.runActive {
			return m, nil
		}
		if cmd, ok := parseSlashCommand(text); ok {
			m.input.Reset()
			m.refreshInputHeight()
			return m, m.handleSlash(cmd)
		}
		return m, m.submitMessage(text)
	}

	var cmds []tea.Cmd
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	cmds = append(cmds, inputCmd)
	m.refreshInputHeight()
	m.updateSlashSuggestions()
	var viewportCmd tea.Cmd
	m.viewport, viewportCmd = m.viewport.Update(msg)
	cmds = append(cmds, viewportCmd)
	return m, tea.Batch(cmds...)
}

func (m *Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalApproval:
		switch msg.String() {
		case "1", "enter":
			if m.runActive {
				m.status = "Waiting for current run to settle..."
				return m, nil
			}
			return m, m.resolveApproval(true)
		case "2":
			if m.runActive {
				m.status = "Waiting for current run to settle..."
				return m, nil
			}
			return m, m.resolveApproval(false)
		case "3", "esc":
			m.closeModal()
			m.status = "Approval kept pending"
			return m, nil
		}
	case modalSessions:
		switch msg.String() {
		case "up", "k":
			if m.sessionIndex > 0 {
				m.sessionIndex--
			}
			return m, nil
		case "down", "j":
			if m.sessionIndex < len(m.sessionItems)-1 {
				m.sessionIndex++
			}
			return m, nil
		case "enter":
			if len(m.sessionItems) == 0 || m.runActive {
				return m, nil
			}
			if err := m.loadSession(m.ctx, m.sessionItems[m.sessionIndex].ID); err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.syncViewport()
			return m, nil
		case "esc":
			m.closeModal()
			return m, nil
		}
	case modalModels:
		switch msg.String() {
		case "up", "k":
			if m.modelIndex > 0 {
				m.modelIndex--
			}
			return m, nil
		case "down", "j":
			if m.modelIndex < len(m.modelItems)-1 {
				m.modelIndex++
			}
			return m, nil
		case "enter":
			if len(m.modelItems) == 0 || m.runActive {
				return m, nil
			}
			return m, m.switchModel(m.modelItems[m.modelIndex], true)
		case "esc":
			m.closeModal()
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) handleSlash(cmd slashCommand) tea.Cmd {
	switch cmd.name {
	case "help":
		m.transcript = append(m.transcript, transcriptBlock{
			kind:  "system",
			title: "System",
			body:  "/new start a new session\n/sessions open the session list\n/resume reopen pending approvals\n/model open the model list\n/model <provider> <model> switch directly",
		})
		m.syncViewport()
		return nil
	case "new", "clear":
		if m.runActive {
			m.status = "Current run is still active"
			return nil
		}
		if err := m.createSession(m.ctx); err != nil {
			m.status = err.Error()
			return nil
		}
		m.transcript = nil
		m.resetStreamingState()
		m.approvalQueue = nil
		m.approvalDecisions = make(map[string]any)
		m.closeModal()
		m.addWelcomeBlock()
		m.status = fmt.Sprintf("Created session %s", m.currentSession.ID())
		m.syncViewport()
		return nil
	case "sessions":
		items, err := m.sessions.List(m.ctx)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		m.sessionItems = items
		m.sessionIndex = 0
		m.setModal(modalSessions)
		return nil
	case "resume":
		if m.runActive {
			m.status = "Current run is still active"
			return nil
		}
		targetSession := m.currentSession
		if len(cmd.args) > 0 {
			if err := m.loadSession(m.ctx, cmd.args[0]); err != nil {
				m.status = err.Error()
				return nil
			}
			targetSession = m.currentSession
		}
		if targetSession == nil {
			m.status = "No current session"
			return nil
		}
		if err := m.loadPendingApprovals(m.ctx); err != nil {
			m.status = err.Error()
			return nil
		}
		if len(m.approvalQueue) == 0 {
			m.status = "No pending approval in current session"
			return nil
		}
		m.setModal(modalApproval)
		return nil
	case "model":
		if len(cmd.args) == 0 {
			providers := m.factory.AvailableProviders()
			m.modelItems = make([]runtime.ModelChoice, 0, len(providers))
			for _, provider := range providers {
				modelName := strings.TrimSpace(m.settings.ModelForProvider(provider))
				if modelName == "" && provider == m.factory.DefaultChoice().Provider {
					modelName = m.factory.DefaultChoice().Name
				}
				if modelName == "" {
					modelName = m.currentModel.Name
				}
				m.modelItems = append(m.modelItems, runtime.ModelChoice{Provider: provider, Name: modelName})
			}
			m.modelIndex = slices.IndexFunc(m.modelItems, func(choice runtime.ModelChoice) bool {
				return choice == m.currentModel
			})
			if m.modelIndex < 0 {
				m.modelIndex = 0
			}
			m.setModal(modalModels)
			return nil
		}
		return m.switchModel(parseModelArgs(cmd.args), false)
	default:
		m.status = fmt.Sprintf("Unknown command: /%s", cmd.name)
		return nil
	}
}

func (m *Model) submitMessage(text string) tea.Cmd {
	if m.currentSession == nil {
		if err := m.createSession(m.ctx); err != nil {
			m.status = fmt.Sprintf("failed to create session: %v", err)
			return nil
		}
	}
	m.transcript = append(m.transcript, transcriptBlock{
		kind:  "user",
		title: "User",
		body:  text,
	})
	m.input.Reset()
	m.refreshInputHeight()
	m.resetStreamingState()
	m.runActive = true
	m.exitRequested = false

	ctx, cancel := context.WithCancel(m.ctx)
	m.runCancel = cancel
	m.status = "Running..."
	m.syncViewport()

	events := m.runner.Run(ctx, m.sessions.UserID, m.currentSession.ID(), message.NewUserMessage(text), agent.RunOptions{
		StreamingMode: agent.StreamingModeSSE,
	})
	return bridge.StartCmd(m.program, events)
}

func (m *Model) resolveApproval(approved bool) tea.Cmd {
	if len(m.approvalQueue) == 0 {
		m.closeModal()
		return nil
	}
	current := m.approvalQueue[0]
	m.approvalQueue = m.approvalQueue[1:]
	m.approvalDecisions[current.InterruptID] = map[string]any{"approved": approved}
	if len(m.approvalQueue) > 0 {
		m.setModal(modalApproval)
		return nil
	}

	m.closeModal()
	m.resetStreamingState()
	m.runActive = true
	m.exitRequested = false
	if m.currentSession != nil {
		if _, err := m.sessions.EnsureCheckpointApprovalDetails(m.ctx, m.currentSession.ID()); err != nil {
			m.runActive = false
			m.status = fmt.Sprintf("failed to prepare resume checkpoint: %v", err)
			return nil
		}
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.runCancel = cancel
	m.status = "Resuming..."
	resumeData := make(map[string]any, len(m.approvalDecisions))
	for k, v := range m.approvalDecisions {
		resumeData[k] = v
	}
	events := m.runner.Run(ctx, m.sessions.UserID, m.currentSession.ID(), nil, agent.RunOptions{
		StreamingMode: agent.StreamingModeSSE,
		ResumeData:    resumeData,
	})
	m.approvalDecisions = make(map[string]any)
	return bridge.StartCmd(m.program, events)
}

func (m *Model) switchModel(choice runtime.ModelChoice, closeModal bool) tea.Cmd {
	choice = m.factory.NormalizeChoice(choice)
	if !choice.Valid() {
		m.status = "Usage: /model <provider> <model>"
		return nil
	}
	if !m.factory.HasProvider(choice.Provider) {
		m.status = fmt.Sprintf("Provider %s is not configured in adk.toml", choice.Provider)
		return nil
	}
	if err := m.rebuildRunnerForChoice(choice); err != nil {
		m.status = err.Error()
		return nil
	}
	if closeModal {
		m.closeModal()
	}
	return nil
}

func (m *Model) rebuildRunnerForChoice(choice runtime.ModelChoice) error {
	prev := m.currentModel
	m.currentModel = choice
	if err := m.rebuildRunner(m.ctx); err != nil {
		m.currentModel = prev
		return err
	}
	m.settings.RememberModel(m.currentModel.Provider, m.currentModel.Name)
	if err := storage.SaveSettings(m.settingsPath, m.settings); err != nil {
		m.status = fmt.Sprintf("model switched, but failed to save settings: %v", err)
	} else {
		m.status = fmt.Sprintf("Switched model to %s", m.currentModel.Label())
	}
	if m.currentSession != nil {
		if err := m.sessions.RecordModelChange(m.ctx, m.currentSession.ID(), m.currentModel, m.currentSession.Entries()); err == nil {
			_ = m.refreshCurrentSession(m.ctx)
		}
	}
	return nil
}

func (m *Model) handleEvent(ev event.Event) {
	if ev == nil {
		return
	}

	switch typed := ev.(type) {
	case *event.TextDelta:
		body := typed.Delta
		if partial := typed.Partial; partial != nil && partial.Text() != "" {
			body = partial.Text()
		}
		m.upsertAssistantText(body)
	case *event.ThinkingDelta:
		body := typed.Delta
		if partial := typed.Partial; partial != nil {
			if thinking := extractThinking(partial); thinking != "" {
				body = thinking
			}
		}
		if body == "" && typed.Redacted {
			body = "(redacted thinking)"
		}
		m.upsertThinking(body)
	case *event.ToolCallDelta:
		body := render.FormatToolCallDelta(typed.ToolName, typed.Delta)
		if partial := typed.Partial; partial != nil {
			if call, ok := toolCallByID(partial, typed.ToolCallID); ok {
				body = render.FormatToolCall(typed.ToolName, call.Args)
			}
		}
		m.upsertToolCall(typed.ToolCallID, typed.ToolName, body)
	case *event.ToolExecutionStart:
		m.upsertToolExecution(typed.ToolCallID, typed.ToolName, render.FormatToolCall(typed.ToolName, typed.Args))
	case *event.ToolExecutionEnd:
		body := render.FormatToolResult(typed.ToolName, typed.Result, typed.IsError)
		m.upsertToolExecution(typed.ToolCallID, typed.ToolName, body)
	case *event.Interrupt:
		if m.currentSession != nil {
			if _, err := m.sessions.EnsureCheckpointApprovalDetails(m.ctx, m.currentSession.ID()); err != nil {
				m.status = fmt.Sprintf("failed to normalize approval checkpoint: %v", err)
			}
		}
		m.approvalQueue = approvalRequestsFromInterrupt(typed.Interrupt)
		m.approvalDecisions = make(map[string]any)
		if len(m.approvalQueue) > 0 {
			m.setModal(modalApproval)
			m.status = fmt.Sprintf("Approval required: %s", m.approvalQueue[0].Tool)
		}
	case *event.MessageEnd:
		if typed.Message == nil {
			break
		}
		switch typed.Message.Role {
		case message.RoleAssistant:
			if text := typed.Message.Text(); text != "" && !typed.Message.HasToolCalls() {
				m.upsertAssistantText(text)
			}
			if thinking := extractThinking(typed.Message); thinking != "" {
				m.upsertThinking(thinking)
			}
			for _, call := range typed.Message.ToolCalls() {
				m.upsertToolCall(call.ID, call.Name, render.FormatValue(call.Args))
			}
		}
	case *event.AgentEnd:
		m.runActive = false
		m.runCancel = nil
		m.exitRequested = false
		m.resetStreamingState()
		if typed.Err != nil {
			if errors.Is(typed.Err, context.Canceled) {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "system",
					title: "System",
					body:  "Current generation was cancelled.",
				})
				m.status = "Cancelled"
			} else {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "error",
					title: "Error",
					body:  typed.Err.Error(),
				})
				m.status = typed.Err.Error()
			}
		} else {
			m.status = "Ready"
		}
		_ = m.refreshCurrentSession(m.ctx)
	}

	m.syncViewport()
}

func (m *Model) applySize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.input.SetWidth(max(20, m.width-2))
	m.refreshInputHeight()
	headerHeight := lipgloss.Height(views.RenderHeader(m.theme, m.width, currentSessionID(m.currentSession), m.currentModel.Label(), m.stateLabel()))
	composerHeight := lipgloss.Height(views.RenderComposer(m.theme, m.width, views.ComposerState{Input: m.input.View(), Status: m.status}))
	modalHeight := 0
	if modal := m.renderModal(); modal != "" {
		modalHeight = lipgloss.Height(views.RenderOverlayDivider(m.theme, m.width)) + lipgloss.Height(modal)
	}
	chatHeight := m.height - headerHeight - composerHeight - modalHeight
	if chatHeight < 5 {
		chatHeight = 5
	}
	m.viewport.Width = m.width
	m.viewport.Height = chatHeight
	m.syncViewport()
}

func (m *Model) syncViewport() {
	if m.viewport.Width <= 0 {
		return
	}
	wasAtBottom := m.viewport.AtBottom() || m.runActive
	m.viewport.SetContent(m.renderTranscript())
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) upsertAssistantText(body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	if m.currentTextBlock < 0 {
		m.currentTextBlock = len(m.transcript)
		m.transcript = append(m.transcript, transcriptBlock{kind: "assistant", title: "Assistant", body: body})
		return
	}
	m.transcript[m.currentTextBlock].body = body
}

func (m *Model) upsertThinking(body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	if m.currentThinkingBlock < 0 {
		m.currentThinkingBlock = len(m.transcript)
		m.transcript = append(m.transcript, transcriptBlock{kind: "thinking", title: "Thinking", body: body})
		return
	}
	m.transcript[m.currentThinkingBlock].body = body
}

func (m *Model) upsertToolCall(toolCallID, toolName, body string) {
	if toolCallID == "" {
		toolCallID = toolName
	}
	index, ok := m.toolCallBlocks[toolCallID]
	if !ok {
		index = len(m.transcript)
		m.toolCallBlocks[toolCallID] = index
		m.transcript = append(m.transcript, transcriptBlock{
			kind:  "tool",
			title: fmt.Sprintf("Tool Call: %s", toolName),
			body:  body,
		})
		return
	}
	m.transcript[index].body = body
}

func (m *Model) upsertToolExecution(toolCallID, toolName, body string) {
	if toolCallID == "" {
		toolCallID = toolName
	}
	index, ok := m.toolExecBlocks[toolCallID]
	if !ok {
		index = len(m.transcript)
		m.toolExecBlocks[toolCallID] = index
		m.transcript = append(m.transcript, transcriptBlock{
			kind:  "tool",
			title: fmt.Sprintf("Tool: %s", toolName),
			body:  body,
		})
		return
	}
	m.transcript[index].body = body
}

func extractThinking(msg *message.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, content := range msg.ThinkingContents() {
		thinking, ok := content.(*message.ThinkingContent)
		if !ok {
			continue
		}
		if thinking.Redacted && thinking.Thinking == "" {
			parts = append(parts, "(redacted thinking)")
			continue
		}
		if thinking.Thinking != "" {
			parts = append(parts, thinking.Thinking)
		}
	}
	return strings.Join(parts, "")
}

func toolCallByID(msg *message.Message, toolCallID string) (message.ToolCall, bool) {
	for _, call := range msg.ToolCalls() {
		if call.ID == toolCallID {
			return call, true
		}
	}
	return message.ToolCall{}, false
}

func approvalRequestsFromInterrupt(data *resume.InterruptData) []approvalRequest {
	if data == nil {
		return nil
	}
	items := make([]approvalRequest, 0, len(data.Contexts))
	seen := make(map[string]struct{})
	for _, ctx := range data.Contexts {
		if ctx == nil {
			continue
		}
		if req, ok := approvalFromAny(ctx.Info, ctx.ID); ok {
			if _, exists := seen[req.InterruptID]; exists {
				continue
			}
			seen[req.InterruptID] = struct{}{}
			items = append(items, req)
		}
	}
	slices.SortFunc(items, func(a, b approvalRequest) int {
		return strings.Compare(a.InterruptID, b.InterruptID)
	})
	return items
}

func approvalFromAny(info any, interruptID string) (approvalRequest, bool) {
	switch typed := info.(type) {
	case interrupttool.ApprovalInfo:
		return approvalRequest{
			InterruptID: interruptID,
			Tool:        typed.Tool,
			Args:        typed.Args,
			Message:     typed.Message,
		}, true
	case *interrupttool.ApprovalInfo:
		if typed == nil {
			return approvalRequest{}, false
		}
		return approvalRequest{
			InterruptID: interruptID,
			Tool:        typed.Tool,
			Args:        typed.Args,
			Message:     typed.Message,
		}, true
	case map[string]any:
		req := approvalRequest{
			InterruptID: interruptID,
			Message:     "Approve this tool call?",
		}
		if toolName, ok := typed["tool"].(string); ok {
			req.Tool = toolName
		}
		if args, ok := typed["args"].(map[string]any); ok {
			req.Args = args
		}
		if messageText, ok := typed["message"].(string); ok && strings.TrimSpace(messageText) != "" {
			req.Message = messageText
		}
		if req.Tool == "" {
			return approvalRequest{}, false
		}
		return req, true
	default:
		return approvalRequest{}, false
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
