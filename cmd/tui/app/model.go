package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nickqiaoo/tadk/cmd/tui/internal/render"
	"github.com/nickqiaoo/tadk/cmd/tui/runtime"
	"github.com/nickqiaoo/tadk/cmd/tui/storage"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
)

type modalType int

const (
	minInputHeight = 1
	maxInputHeight = 6
)

const (
	modalNone modalType = iota
	modalApproval
	modalSessions
	modalModels
)

type transcriptBlock struct {
	kind  string
	title string
	body  string
}

type approvalRequest struct {
	InterruptID string
	Tool        string
	Args        map[string]any
	Message     string
}

type Deps struct {
	Context      context.Context
	Factory      *runtime.Factory
	Sessions     *runtime.SessionManager
	Settings     storage.Settings
	SettingsPath string
	SessionID    string
}

type Model struct {
	ctx    context.Context
	cancel context.CancelFunc

	program *tea.Program

	width  int
	height int

	viewport viewport.Model
	input    textarea.Model
	theme    render.Theme

	factory      *runtime.Factory
	sessions     *runtime.SessionManager
	settings     storage.Settings
	settingsPath string

	currentModel   runtime.ModelChoice
	currentSession session.Session
	runner         *runner.Runner

	transcript []transcriptBlock
	status     string

	modal        modalType
	sessionItems []runtime.SessionSummary
	sessionIndex int
	modelItems   []runtime.ModelChoice
	modelIndex   int

	approvalQueue     []approvalRequest
	approvalDecisions map[string]any

	runActive     bool
	runCancel     context.CancelFunc
	exitRequested bool

	currentTextBlock     int
	currentThinkingBlock int
	toolCallBlocks       map[string]int
	toolExecBlocks       map[string]int

	slashSuggestions      []slashSuggestion
	slashSelectedIdx      int
	slashSuggestionsActive bool
}

func New(deps Deps) (*Model, error) {
	rootCtx := deps.Context
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(rootCtx)

	input := textarea.New()
	input.Prompt = "> "
	input.Placeholder = "Type a message or / for commands"
	input.ShowLineNumbers = false
	input.SetHeight(minInputHeight)
	input.CharLimit = 0
	input.MaxHeight = maxInputHeight
	const composerFg = "#e0e0e0"
	const composerMuted = "#666666"
	const composerAccent = "#b1b9f9"
	input.FocusedStyle.Base = lipgloss.NewStyle().Foreground(lipgloss.Color(composerFg))
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(composerFg))
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(composerAccent)).Bold(true)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(composerMuted))
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color(composerMuted))
	input.BlurredStyle = input.FocusedStyle
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Cursor.Style = lipgloss.NewStyle().Background(lipgloss.Color(composerAccent))

	m := &Model{
		ctx:                  ctx,
		cancel:               cancel,
		viewport:             viewport.New(0, 0),
		input:                input,
		theme:                render.NewTheme(),
		factory:              deps.Factory,
		sessions:             deps.Sessions,
		settings:             deps.Settings,
		settingsPath:         deps.SettingsPath,
		currentTextBlock:     -1,
		currentThinkingBlock: -1,
		toolCallBlocks:       make(map[string]int),
		toolExecBlocks:       make(map[string]int),
		approvalDecisions:    make(map[string]any),
		status:               "Ready",
		slashSuggestions:     defaultSlashSuggestions(),
	}
	m.viewport.MouseWheelEnabled = true
	m.viewport.MouseWheelDelta = 6

	m.currentModel = m.initialChoice()

	var err error
	if strings.TrimSpace(deps.SessionID) != "" {
		m.currentSession, err = m.sessions.Load(ctx, deps.SessionID)
		if err != nil {
			return nil, err
		}
		if choice, ok := runtime.SessionModelChoice(m.currentSession); ok && m.factory.HasProvider(choice.Provider) {
			m.currentModel = choice
		}
	}

	if err := m.rebuildRunner(ctx); err != nil {
		return nil, err
	}

	if m.currentSession != nil {
		m.rebuildTranscriptFromSession()
		if err := m.loadPendingApprovals(ctx); err != nil {
			return nil, err
		}
	}

	if len(m.transcript) == 0 {
		m.addWelcomeBlock()
	}
	m.refreshInputHeight()

	return m, nil
}

func (m *Model) SetProgram(program *tea.Program) {
	m.program = program
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.WindowSize(),
		m.input.Focus(),
	)
}

func (m *Model) initialChoice() runtime.ModelChoice {
	choice := m.factory.DefaultChoice()
	if provider := strings.TrimSpace(strings.ToLower(m.settings.SelectedProvider)); provider != "" && m.factory.HasProvider(provider) {
		choice.Provider = provider
	}
	if modelName := strings.TrimSpace(m.settings.SelectedModel); modelName != "" {
		choice.Name = modelName
	}
	if remembered := strings.TrimSpace(m.settings.ModelForProvider(choice.Provider)); remembered != "" {
		choice.Name = remembered
	}
	return m.factory.NormalizeChoice(choice)
}

func (m *Model) rebuildRunner(ctx context.Context) error {
	bundle, err := m.factory.Build(ctx, m.currentModel, m.sessions.Service, storage.AppName)
	if err != nil {
		return err
	}
	m.runner = bundle.Runner
	m.currentModel = bundle.Choice
	return nil
}

func (m *Model) createSession(ctx context.Context) error {
	sess, err := m.sessions.New(ctx, "")
	if err != nil {
		return err
	}
	m.currentSession = sess
	if err := m.sessions.RecordModelChange(ctx, m.currentSession.ID(), m.currentModel, m.currentSession.Entries()); err != nil {
		return err
	}
	return m.refreshCurrentSession(ctx)
}

func (m *Model) refreshCurrentSession(ctx context.Context) error {
	if m.currentSession == nil {
		return nil
	}
	sess, err := m.sessions.Load(ctx, m.currentSession.ID())
	if err != nil {
		return err
	}
	m.currentSession = sess
	return nil
}

func (m *Model) loadSession(ctx context.Context, sessionID string) error {
	sess, err := m.sessions.Load(ctx, sessionID)
	if err != nil {
		return err
	}
	if choice, ok := runtime.SessionModelChoice(sess); ok && m.factory.HasProvider(choice.Provider) {
		if choice != m.currentModel {
			m.currentModel = choice
			if err := m.rebuildRunner(ctx); err != nil {
				return err
			}
		}
	}
	m.currentSession = sess
	m.closeModal()
	m.resetStreamingState()
	m.approvalQueue = nil
	m.approvalDecisions = make(map[string]any)
	m.rebuildTranscriptFromSession()
	return m.loadPendingApprovals(ctx)
}

func (m *Model) loadPendingApprovals(ctx context.Context) error {
	m.approvalQueue = nil
	m.approvalDecisions = make(map[string]any)
	if m.currentSession == nil {
		return nil
	}
	cp, err := m.sessions.EnsureCheckpointApprovalDetails(ctx, m.currentSession.ID())
	if err != nil || cp == nil || len(cp.PendingInterrupts) == 0 {
		return err
	}
	ids := runtime.SortedPendingInterruptIDs(cp)
	for _, id := range ids {
		pending := cp.PendingInterrupts[id]
		if pending == nil {
			continue
		}
		req := approvalRequest{
			InterruptID: id,
			Tool:        pending.ToolName,
			Args:        pending.ToolArgs,
			Message:     "Approve this tool call?",
		}
		if approval, ok := approvalFromAny(pending.Info, id); ok {
			req = approval
		}
		m.approvalQueue = append(m.approvalQueue, req)
	}
	if len(m.approvalQueue) > 0 {
		m.setModal(modalApproval)
		m.status = fmt.Sprintf("Session %s has pending approval", m.currentSession.ID())
	}
	return nil
}

func (m *Model) setModal(next modalType) {
	if m.modal == next {
		return
	}
	m.modal = next
	m.applySize()
}

func (m *Model) closeModal() {
	m.setModal(modalNone)
}

func (m *Model) resetStreamingState() {
	m.currentTextBlock = -1
	m.currentThinkingBlock = -1
	m.toolCallBlocks = make(map[string]int)
	m.toolExecBlocks = make(map[string]int)
}

func (m *Model) addWelcomeBlock() {
	m.transcript = append(m.transcript, transcriptBlock{
		kind:  "banner",
		title: "",
		body:  render.Banner(),
	})
	m.transcript = append(m.transcript, transcriptBlock{
		kind:  "system",
		title: "System",
		body:  "tadk TUI is ready. Type a message to start, or use /help for commands.",
	})
}

func (m *Model) refreshInputHeight() {
	height := minInputHeight
	value := m.input.Value()
	if value != "" {
		available := m.input.Width() - len([]rune(m.input.Prompt))
		if available <= 0 {
			available = 1
		}
		height = 0
		for _, line := range strings.Split(value, "\n") {
			lineWidth := len([]rune(line))
			rows := 1
			if lineWidth > 0 {
				rows = (lineWidth + available - 1) / available
			}
			height += rows
		}
		if height < minInputHeight {
			height = minInputHeight
		}
	}
	if height > maxInputHeight {
		height = maxInputHeight
	}
	m.input.SetHeight(height)
}

func (m *Model) rebuildTranscriptFromSession() {
	m.transcript = nil
	if m.currentSession == nil {
		return
	}

	for _, entry := range runtime.CollectEntries(m.currentSession.Entries()) {
		msgEntry, ok := entry.(*session.MessageEntry)
		if !ok || msgEntry == nil || msgEntry.Message == nil {
			continue
		}

		msg := msgEntry.Message
		switch msg.Role {
		case message.RoleUser:
			text := strings.TrimSpace(msg.Text())
			if text != "" {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "user",
					title: "User",
					body:  text,
				})
			}
		case message.RoleAssistant:
			if thinking := extractThinking(msg); strings.TrimSpace(thinking) != "" {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "thinking",
					title: "Thinking",
					body:  thinking,
				})
			}
			if text := strings.TrimSpace(msg.Text()); text != "" {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "assistant",
					title: "Assistant",
					body:  text,
				})
			}
			for _, call := range msg.ToolCalls() {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "tool",
					title: fmt.Sprintf("Tool Call: %s", call.Name),
					body:  render.FormatToolCall(call.Name, call.Args),
				})
			}
		case message.RoleToolResult:
			results := msg.ToolResults()
			if len(results) == 0 && strings.TrimSpace(msg.Text()) != "" {
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "tool",
					title: "Tool Result",
					body:  msg.Text(),
				})
				continue
			}
			for _, result := range results {
				title := "Tool Result"
				if result.Name != "" {
					title = fmt.Sprintf("Tool: %s", result.Name)
				}
				m.transcript = append(m.transcript, transcriptBlock{
					kind:  "tool",
					title: title,
					body:  render.FormatToolResult(result.Name, result.Content, false),
				})
			}
		}
	}
}

type slashSuggestion struct {
	name        string
	description string
}

func defaultSlashSuggestions() []slashSuggestion {
	return []slashSuggestion{
		{name: "help", description: "Show available commands"},
		{name: "new", description: "Start a new session"},
		{name: "clear", description: "Clear the current session"},
		{name: "sessions", description: "Open the session list"},
		{name: "resume", description: "Reopen pending approvals"},
		{name: "model", description: "Open the model list or switch"},
	}
}

func (m *Model) updateSlashSuggestions() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") {
		m.slashSuggestionsActive = false
		m.slashSelectedIdx = 0
		return
	}
	// if there is a space after the command, hide suggestions
	if strings.Contains(strings.TrimPrefix(val, "/"), " ") {
		m.slashSuggestionsActive = false
		m.slashSelectedIdx = 0
		return
	}
	prefix := strings.ToLower(strings.TrimPrefix(val, "/"))
	var matched []slashSuggestion
	for _, s := range defaultSlashSuggestions() {
		if strings.HasPrefix(s.name, prefix) {
			matched = append(matched, s)
		}
	}
	m.slashSuggestions = matched
	m.slashSuggestionsActive = len(matched) > 0
	if m.slashSelectedIdx >= len(matched) {
		m.slashSelectedIdx = 0
	}
}

func (m *Model) acceptSlashSuggestion() {
	if !m.slashSuggestionsActive || m.slashSelectedIdx >= len(m.slashSuggestions) {
		return
	}
	cmd := "/" + m.slashSuggestions[m.slashSelectedIdx].name + " "
	m.input.SetValue(cmd)
	m.input.SetCursor(len([]rune(cmd)))
	m.slashSuggestionsActive = false
	m.slashSelectedIdx = 0
	m.refreshInputHeight()
}
