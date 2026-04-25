package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

func TestServiceWritesEntryJSONL(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}

	ctx := t.Context()
	created, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	entry := session.NewMessageLogEntry("", message.NewAssistantMessage("hello"), "agent", "inv-1", "root")
	if err := svc.AppendEntry(ctx, &session.AppendEntryRequest{
		AppName:   created.Session.AppName(),
		UserID:    created.Session.UserID(),
		SessionID: created.Session.ID(),
		Entry:     entry,
	}); err != nil {
		t.Fatalf("AppendEntry(): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "app", "user", "sess", "entries.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("JSONL line count = %d, want 1", len(lines))
	}
	var probe struct {
		Type session.EntryType `json:"type"`
		Kind string            `json:"kind"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &probe); err != nil {
		t.Fatalf("json.Unmarshal(probe): %v", err)
	}
	if probe.Type != session.EntryTypeMessage {
		t.Fatalf("probe.Type = %q, want %q", probe.Type, session.EntryTypeMessage)
	}
	if probe.Kind != "" {
		t.Fatalf("probe.Kind = %q, want empty legacy record kind", probe.Kind)
	}

	got, err := svc.Get(ctx, &session.GetRequest{AppName: "app", UserID: "user", SessionID: "sess"})
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Session.Entries().Len() != 1 {
		t.Fatalf("Entries().Len() = %d, want 1", got.Session.Entries().Len())
	}
	gotEntry, ok := got.Session.Entries().At(0).(*session.MessageEntry)
	if !ok {
		t.Fatalf("got entry is not a MessageEntry")
	}
	if gotEntry.Message.Text() != "hello" {
		t.Fatalf("Message.Text() = %q, want hello", gotEntry.Message.Text())
	}
}

// TestSessionLiveView verifies that a Session obtained via Get reflects
// subsequent AppendEntry writes. This is the regression test for the bug
// where Gemini re-read the same files every turn because invCtx.Session()
// held a stale snapshot.
func TestSessionLiveView(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	ctx := t.Context()
	if _, err := svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user", SessionID: "sess"}); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	resp, err := svc.Get(ctx, &session.GetRequest{AppName: "app", UserID: "user", SessionID: "sess"})
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	sess := resp.Session
	if sess.Entries().Len() != 0 {
		t.Fatalf("initial Entries().Len() = %d, want 0", sess.Entries().Len())
	}

	entry := session.NewMessageLogEntry("", message.NewAssistantMessage("first"), "agent", "inv-1", "root")
	if err := svc.AppendEntry(ctx, &session.AppendEntryRequest{AppName: "app", UserID: "user", SessionID: "sess", Entry: entry}); err != nil {
		t.Fatalf("AppendEntry(): %v", err)
	}
	if sess.Entries().Len() != 1 {
		t.Fatalf("after append Entries().Len() = %d, want 1 (session should be live)", sess.Entries().Len())
	}
}

func TestServiceCheckpoint(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := svc.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	ctx := t.Context()
	created, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", ".lock")); err != nil {
		t.Fatalf(".lock stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "entries.jsonl")); err != nil {
		t.Fatalf("entries.jsonl stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "meta.json")); !os.IsNotExist(err) {
		t.Fatalf("meta.json should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest.json should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "owner.json")); !os.IsNotExist(err) {
		t.Fatalf("owner.json should not exist, stat err = %v", err)
	}
	if created.Session.ID() != "sess" {
		t.Fatalf("Session.ID() = %q, want sess", created.Session.ID())
	}

	if err := svc.SaveCheckpoint(ctx, &session.SaveCheckpointRequest{
		CheckpointRequest: session.CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"},
		Checkpoint: &session.Checkpoint{
			Status:       session.CheckpointStatusInterrupted,
			InvocationID: "inv-1",
			PendingInterrupts: map[string]*session.PendingInterrupt{
				"interrupt-1": {
					ID:           "interrupt-1",
					AgentAddress: "root.child",
					ToolCallID:   "call-1",
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveCheckpoint(): %v", err)
	}

	checkpoint, err := svc.GetCheckpoint(ctx, &session.CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"})
	if err != nil {
		t.Fatalf("GetCheckpoint(): %v", err)
	}
	if checkpoint == nil {
		t.Fatal("checkpoint is nil")
	}
	if checkpoint.Revision != 1 {
		t.Fatalf("checkpoint.Revision = %d, want 1", checkpoint.Revision)
	}
	if checkpoint.PendingInterrupts["interrupt-1"].ToolCallID != "call-1" {
		t.Fatalf("ToolCallID = %q, want call-1", checkpoint.PendingInterrupts["interrupt-1"].ToolCallID)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "agents", "root.child", "checkpoint.json")); !os.IsNotExist(err) {
		t.Fatalf("per-agent checkpoint mirror should not exist, stat err = %v", err)
	}

	if err := svc.ClearCheckpoint(ctx, &session.CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"}); err != nil {
		t.Fatalf("ClearCheckpoint(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "checkpoint.json")); !os.IsNotExist(err) {
		t.Fatalf("checkpoint.json after clear stat err = %v, want not exist", err)
	}
}

// TestServiceLockConflict verifies flock rejects concurrent openers.
func TestServiceLockConflict(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(first): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := svc.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	ctx := t.Context()
	if _, err := svc.Create(ctx, &session.CreateRequest{AppName: "app", UserID: "user", SessionID: "sess"}); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	other, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(second): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := other.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	if _, err := other.Get(ctx, &session.GetRequest{AppName: "app", UserID: "user", SessionID: "sess"}); err == nil {
		t.Fatal("other service Get() succeeded, want lock conflict")
	}
}

func TestServiceAgentState(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}

	ctx := t.Context()
	_, err = svc.Create(ctx, &session.CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	err = svc.SaveAgentState(ctx, &session.SaveAgentStateRequest{
		AgentStateRequest: session.AgentStateRequest{
			AppName:      "app",
			UserID:       "user",
			SessionID:    "sess",
			AgentAddress: "root.child",
		},
		State: &session.AgentState{
			Data: map[string]any{
				"iteration": 3,
				"cursor":    "step-2",
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveAgentState(): %v", err)
	}

	got, err := svc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "app",
		UserID:       "user",
		SessionID:    "sess",
		AgentAddress: "root.child",
	})
	if err != nil {
		t.Fatalf("GetAgentState(): %v", err)
	}
	if got == nil {
		t.Fatal("GetAgentState() = nil, want state")
	}
	if got.Data["iteration"] != float64(3) && got.Data["iteration"] != 3 {
		t.Fatalf("iteration = %#v, want 3", got.Data["iteration"])
	}
	if got.AgentAddress != "root.child" {
		t.Fatalf("AgentAddress = %q, want root.child", got.AgentAddress)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "agents", "root.child", "state.json")); err != nil {
		t.Fatalf("agent state stat: %v", err)
	}

	if err := svc.ClearAgentState(ctx, &session.AgentStateRequest{
		AppName:      "app",
		UserID:       "user",
		SessionID:    "sess",
		AgentAddress: "root.child",
	}); err != nil {
		t.Fatalf("ClearAgentState(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "user", "sess", "agents", "root.child", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("agent state after clear stat err = %v, want not exist", err)
	}
}
