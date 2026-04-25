package message

import (
	"encoding/json"
	"testing"
)

func TestMessageJSONRoundTripPreservesContentAndToolResults(t *testing.T) {
	original := &Message{
		Role: RoleAssistant,
		Content: []Content{
			&TextContent{Text: "hello"},
			&ThinkingContent{Thinking: "thinking", ThinkingSignature: "sig"},
			&ToolCallContent{ID: "call-1", Name: "search", Arguments: map[string]any{"q": "adk"}},
			&ImageContent{MimeType: "image/png", Data: string([]byte{1, 2, 3})},
		},
	}
	toolResultOriginal := NewToolResultMessages([]ToolResult{{
		CallID:  "call-1",
		Name:    "search",
		Content: map[string]any{"ok": true},
	}}, nil)

	for _, original := range []*Message{original, toolResultOriginal} {
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal() error: %v", err)
		}

		var decoded Message
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error: %v", err)
		}

		if decoded.Role != original.Role {
			t.Fatalf("Role = %q, want %q", decoded.Role, original.Role)
		}
		if len(decoded.Content) != len(original.Content) {
			t.Fatalf("len(Content) = %d, want %d", len(decoded.Content), len(original.Content))
		}
		if len(decoded.ToolResults()) != len(original.ToolResults()) {
			t.Fatalf("len(ToolResults) = %d, want %d", len(decoded.ToolResults()), len(original.ToolResults()))
		}
	}
}
