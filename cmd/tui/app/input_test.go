package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nickqiaoo/tadk/cmd/tui/runtime"
	"github.com/nickqiaoo/tadk/cmd/tui/storage"
	"github.com/nickqiaoo/tadk/cmd/tui/tools"
	"github.com/nickqiaoo/tadk/session"
)

func newTestModel(t *testing.T) *Model {
	t.Helper()

	settingsPath, err := storage.SettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := storage.LoadSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	factory, err := runtime.NewFactory("testdata/config.toml", tools.Registry())
	if err != nil {
		t.Fatal(err)
	}
	sessionsDir, err := storage.SessionsDir()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := runtime.NewSessionManager(storage.AppName, storage.DefaultUserID(), sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })

	m, err := New(Deps{
		Context:      context.Background(),
		Factory:      factory,
		Sessions:     sessions,
		Settings:     settings,
		SettingsPath: settingsPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if m.currentSession != nil {
			_ = sessions.Service.Delete(context.Background(), &session.DeleteRequest{
				AppName:   sessions.AppName,
				UserID:    sessions.UserID,
				SessionID: m.currentSession.ID(),
			})
		}
	})
	return m
}

func TestInputStartsFocused(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()
	if !m.input.Focused() {
		t.Fatal("expected input to be focused after init")
	}
}

func TestTypingUpdatesTextarea(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.modal != modalNone {
		t.Fatalf("expected modalNone before typing, got %v", m.modal)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if got := m.input.Value(); got != "hi" {
		t.Fatalf("expected input value to be hi, got %q", got)
	}
}

func TestTextareaDirectUpdateReceivesRunes(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if cmd != nil {
		_ = cmd
	}
	if got := m.input.Value(); got != "h" {
		t.Fatalf("expected direct textarea update to produce h, got %q", got)
	}
}

func TestHandleKeyUpdatesTextarea(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if got := m.input.Value(); got != "h" {
		t.Fatalf("expected handleKey to produce h, got %q", got)
	}
}

func TestCtrlCRequiresTwoPressesToQuitFromModal(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()
	m.setModal(modalSessions)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected first ctrl+c to arm exit only")
	}
	if !m.exitRequested {
		t.Fatal("expected exitRequested to be set")
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestEscCancelsRun(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()

	called := 0
	m.runActive = true
	m.runCancel = func() { called++ }

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("expected esc to cancel, not quit")
	}
	if called != 1 {
		t.Fatalf("expected cancel func to be called once, got %d", called)
	}
	if m.exitRequested {
		t.Fatal("did not expect exitRequested to be set")
	}
}

func TestCtrlCQuitsOnSecondPressDuringRun(t *testing.T) {
	m := newTestModel(t)
	_ = m.Init()

	called := 0
	var cmd tea.Cmd
	m.runActive = true
	m.runCancel = func() { called++ }

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected first ctrl+c to arm exit only")
	}
	if called != 0 {
		t.Fatalf("did not expect runCancel on ctrl+c, got %d calls", called)
	}
	if !m.exitRequested {
		t.Fatal("expected exitRequested to be set")
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected second ctrl+c to quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}
