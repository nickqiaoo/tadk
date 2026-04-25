package session

import (
	"encoding/json"
	"testing"

	"github.com/nickqiaoo/tadk/message"
)

func TestMessageEntryRoundTrip(t *testing.T) {
	entry := NewMessageLogEntry("parent-1", message.NewAssistantMessage("hello"), "agent", "inv-1", "root.child")
	entry.ID = "entry-1"
	entry.ErrorCode = "boom"

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal(entry): %v", err)
	}
	var decoded MessageEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(entry): %v", err)
	}
	if decoded.Type != EntryTypeMessage {
		t.Fatalf("Type = %q, want %q", decoded.Type, EntryTypeMessage)
	}
	if decoded.ParentID != "parent-1" {
		t.Fatalf("ParentID = %q, want parent-1", decoded.ParentID)
	}
	if decoded.Message.Text() != "hello" {
		t.Fatalf("Message.Text() = %q, want hello", decoded.Message.Text())
	}
	if decoded.ErrorCode != "boom" {
		t.Fatalf("ErrorCode = %q, want boom", decoded.ErrorCode)
	}
}
