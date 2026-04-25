package utils

import (
	"testing"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

func TestExtractToolCallsKeepsGenericTransferTool(t *testing.T) {
	msg := &message.Message{
		Role: message.RoleAssistant,
		Content: []message.Content{
			&message.ToolCallContent{ID: "call-1", Name: GenericTransferToolName, Arguments: map[string]any{"agent_name": "writer"}},
			&message.ToolCallContent{ID: "call-2", Name: TransferToolPrefix + "reviewer"},
			&message.ToolCallContent{ID: "call-3", Name: "lookup"},
		},
	}

	calls := ExtractToolCalls(msg)
	if len(calls) != 2 {
		t.Fatalf("ExtractToolCalls returned %d calls, want 2", len(calls))
	}
	if calls[0].Name != GenericTransferToolName {
		t.Fatalf("first call name = %q, want %q", calls[0].Name, GenericTransferToolName)
	}
	if calls[1].Name != "lookup" {
		t.Fatalf("second call name = %q, want lookup", calls[1].Name)
	}
}

func TestExtractTransferTargetUsesTargetSpecificConvention(t *testing.T) {
	msg := &message.Message{
		Role: message.RoleAssistant,
		Content: []message.Content{
			&message.ToolCallContent{ID: "call-1", Name: GenericTransferToolName, Arguments: map[string]any{"agent_name": "writer"}},
			&message.ToolCallContent{ID: "call-2", Name: TransferToolPrefix + "reviewer"},
		},
	}

	if target := ExtractTransferTarget(msg); target != "reviewer" {
		t.Fatalf("ExtractTransferTarget = %q, want reviewer", target)
	}
}

func TestResumeDataForToolCallAcceptsInterruptAndLegacyCallIDs(t *testing.T) {
	if data, ok := ResumeDataForToolCall(map[string]any{"interrupt-1": "durable"}, "call-1", "interrupt-1"); !ok || data != "durable" {
		t.Fatalf("ResumeDataForToolCall durable data = %v, %v; want durable, true", data, ok)
	}
	if data, ok := ResumeDataForToolCall(map[string]any{"call-1": "legacy"}, "call-1", "interrupt-1"); !ok || data != "legacy" {
		t.Fatalf("ResumeDataForToolCall legacy data = %v, %v; want legacy, true", data, ok)
	}
}

func TestBuildHistoryUsesEntryBranch(t *testing.T) {
	entries := []session.DurableEntry{
		&session.MessageEntry{
			EntryBase: session.EntryBase{Type: session.EntryTypeMessage},
			Branch:    "root",
			Message:   message.NewUserMessage("root"),
		},
		&session.MessageEntry{
			EntryBase: session.EntryBase{Type: session.EntryTypeMessage},
			Branch:    "root.child",
			Message:   message.NewAssistantMessage("child"),
		},
		&session.MessageEntry{
			EntryBase: session.EntryBase{Type: session.EntryTypeMessage},
			Branch:    "peer",
			Message:   message.NewAssistantMessage("peer"),
		},
	}

	history := BuildHistory(entries, "root.child")
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2", len(history))
	}
	if history[0].Text() != "root" || history[1].Text() != "child" {
		t.Fatalf("history text = %q, %q; want root, child", history[0].Text(), history[1].Text())
	}
}
