package stream

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
)

// partTypeName returns the Go type name without package prefix.
func partTypeName(p Part) string {
	name := fmt.Sprintf("%T", p)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func assertPartTypes(t *testing.T, parts []Part, expected ...string) {
	t.Helper()
	if len(parts) != len(expected) {
		names := make([]string, len(parts))
		for i, p := range parts {
			names[i] = partTypeName(p)
		}
		t.Fatalf("expected %d parts %v, got %d: %v", len(expected), expected, len(parts), names)
	}
	for i, name := range expected {
		got := partTypeName(parts[i])
		if got != name {
			t.Errorf("parts[%d] = %s, want %s", i, got, name)
		}
	}
}

func assertContains(t *testing.T, parts []Part, typeName string) {
	t.Helper()
	for _, p := range parts {
		if partTypeName(p) == typeName {
			return
		}
	}
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = partTypeName(p)
	}
	t.Errorf("parts %v does not contain %s", names, typeName)
}

func assertNotContains(t *testing.T, parts []Part, typeName string) {
	t.Helper()
	for _, p := range parts {
		if partTypeName(p) == typeName {
			t.Errorf("parts should not contain %s", typeName)
			return
		}
	}
}

func textContent(text string) message.Content {
	return &message.TextContent{
 Text: text}
}

func thinkingContent(text, signature string) message.Content {
	return &message.ThinkingContent{
 Thinking: text, ThinkingSignature: signature}
}

func toolCallContent(id, name string, args map[string]any) message.Content {
	return &message.ToolCallContent{
 ID: id, Name: name, Arguments: args}
}

func toolResultMessage(callID, name string, content any, isError bool, extraContent ...message.Content) *message.Message {
	return message.NewToolResultMessages([]message.ToolResult{{
		CallID:  callID,
		Name:    name,
		Content: content,
		IsError: isError,
	}}, extraContent)
}

func TestEntryToParts_TextStreaming(t *testing.T) {
	state := NewStreamState()

	// Complete entry: "Hello, world!"
	ev := &session.MessageEntry{
		EntryBase:  session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		StopReason: model.StopReasonStop,
		Usage:      &model.Usage{InputTokens: 10, OutputTokens: 5},
		Message: &message.Message{
			Role:    message.RoleAssistant,
			Content: []message.Content{textContent("Hello, world!")},
		},
	}
	parts := EntryToParts(ev, state)
	// Start + StartStep + TextStart + TextDelta + TextEnd + FinishStep + Finish
	assertPartTypes(t, parts, "Start", "StartStep", "TextStart", "TextDelta", "TextEnd", "FinishStep", "Finish")
	if parts[3].(TextDelta).Delta != "Hello, world!" {
		t.Errorf("delta = %q, want 'Hello, world!'", parts[3].(TextDelta).Delta)
	}
	if parts[6].(Finish).FinishReason != FinishReasonStop {
		t.Errorf("finishReason = %q, want 'stop'", parts[6].(Finish).FinishReason)
	}
}

func TestEntryToParts_ToolCallAndResult(t *testing.T) {
	state := NewStreamState()

	// Assistant message with text + tool call
	ev1 := &session.MessageEntry{
		EntryBase:  session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		StopReason: model.StopReasonToolUse,
		Message: &message.Message{
			Role: message.RoleAssistant,
			Content: []message.Content{
				textContent("Let me check..."),
				toolCallContent("call_1", "bash", map[string]any{"cmd": "ls"}),
			},
		},
	}
	parts1 := EntryToParts(ev1, state)

	assertContains(t, parts1, "Start")
	assertContains(t, parts1, "StartStep")
	assertContains(t, parts1, "TextStart")
	assertContains(t, parts1, "TextDelta")
	assertContains(t, parts1, "TextEnd")
	assertContains(t, parts1, "ToolInputStart")
	assertContains(t, parts1, "ToolInputAvailable")
	assertContains(t, parts1, "FinishStep")
	// No Finish because tool calls mean the step continues
	assertNotContains(t, parts1, "Finish")

	// Verify tool input
	for _, p := range parts1 {
		if tia, ok := p.(ToolInputAvailable); ok {
			if tia.ToolCallID != "call_1" {
				t.Errorf("toolCallId = %q, want 'call_1'", tia.ToolCallID)
			}
			if tia.ToolName != "bash" {
				t.Errorf("toolName = %q, want 'bash'", tia.ToolName)
			}
		}
	}

	// Tool result message
	ev2 := &session.MessageEntry{
		EntryBase: session.EntryBase{Type: session.EntryTypeMessage, ID: "ev2"},
		Message:   toolResultMessage("call_1", "", map[string]any{"output": "file.txt"}, false),
	}
	parts2 := EntryToParts(ev2, state)
	assertContains(t, parts2, "ToolOutputAvailable")
}

func TestEntryToParts_ToolCallOnlyEmitsOnce(t *testing.T) {
	state := NewStreamState()

	ev := &session.MessageEntry{
		EntryBase:  session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		StopReason: model.StopReasonToolUse,
		Message: &message.Message{
			Role:    message.RoleAssistant,
			Content: []message.Content{toolCallContent("call_1", "bash", map[string]any{"cmd": "ls"})},
		},
	}
	parts := EntryToParts(ev, state)
	assertContains(t, parts, "Start")
	assertContains(t, parts, "StartStep")
	assertContains(t, parts, "ToolInputStart")
	assertContains(t, parts, "ToolInputAvailable")
	assertContains(t, parts, "FinishStep")
	assertNotContains(t, parts, "Finish")
}

func TestEntryToParts_NonStreamingResponse(t *testing.T) {
	state := NewStreamState()

	// Single complete event
	ev := &session.MessageEntry{
		EntryBase:  session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		StopReason: model.StopReasonStop,
		Message: &message.Message{
			Role:    message.RoleAssistant,
			Content: []message.Content{textContent("Hello!")},
		},
	}
	parts := EntryToParts(ev, state)
	// Start + StartStep + TextStart + TextDelta + TextEnd + FinishStep + Finish
	assertPartTypes(t, parts,
		"Start", "StartStep", "TextStart", "TextDelta", "TextEnd", "FinishStep", "Finish")

	if parts[3].(TextDelta).Delta != "Hello!" {
		t.Errorf("delta = %q, want 'Hello!'", parts[3].(TextDelta).Delta)
	}
}

func TestEntryToParts_ToolResultError(t *testing.T) {
	state := NewStreamState()

	ev := &session.MessageEntry{
		EntryBase: session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		Message:   toolResultMessage("call_1", "", "command failed", true),
	}
	parts := EntryToParts(ev, state)
	assertContains(t, parts, "ToolOutputError")

	for _, p := range parts {
		if toe, ok := p.(ToolOutputError); ok {
			if toe.ToolCallID != "call_1" {
				t.Errorf("toolCallId = %q, want 'call_1'", toe.ToolCallID)
			}
			if toe.ErrorText != "command failed" {
				t.Errorf("errorText = %q, want 'command failed'", toe.ErrorText)
			}
		}
	}
}

func TestEntryToParts_ThinkingDelta(t *testing.T) {
	state := NewStreamState()

	ev := &session.MessageEntry{
		EntryBase: session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		Message: &message.Message{
			Role: message.RoleAssistant,
			Content: []message.Content{
				thinkingContent("I need to consider...", ""),
				textContent(""),
			},
		},
	}
	parts := EntryToParts(ev, state)

	assertContains(t, parts, "ReasoningStart")
	assertContains(t, parts, "ReasoningDelta")
	assertContains(t, parts, "ReasoningEnd")

	for _, p := range parts {
		if rd, ok := p.(ReasoningDelta); ok {
			if rd.Delta != "I need to consider..." {
				t.Errorf("reasoning delta = %q, want 'I need to consider...'", rd.Delta)
			}
		}
	}
}

func TestEntryToParts_ThinkingSignature(t *testing.T) {
	state := NewStreamState()

	// Complete with signature
	ev := &session.MessageEntry{
		EntryBase: session.EntryBase{Type: session.EntryTypeMessage, ID: "ev1"},
		Message: &message.Message{
			Role: message.RoleAssistant,
			Content: []message.Content{
				thinkingContent("thinking...", "sig123"),
				textContent("Here's my answer"),
			},
		},
		StopReason: model.StopReasonStop,
	}
	parts := EntryToParts(ev, state)

	// Should include ReasoningEnd with providerMetadata
	for _, p := range parts {
		if re, ok := p.(ReasoningEnd); ok {
			if re.ProviderMetadata == nil {
				t.Fatal("ReasoningEnd should have providerMetadata")
			}
			anthro, ok := re.ProviderMetadata["anthropic"].(map[string]any)
			if !ok {
				t.Fatal("missing anthropic key in providerMetadata")
			}
			if anthro["signature"] != "sig123" {
				t.Errorf("signature = %v, want 'sig123'", anthro["signature"])
			}
			return
		}
	}
	t.Error("missing ReasoningEnd")
}

func TestEntryToParts_CustomEntryIgnored(t *testing.T) {
	state := NewStreamState()

	ev := &session.CustomEntry{
		EntryBase:  session.NewEntryBase(session.EntryTypeCustom, "ev1"),
		CustomType: "test",
		Data:       map[string]any{"key": "value"},
	}
	parts := EntryToParts(ev, state)
	if parts != nil {
		t.Errorf("expected nil for non-message entry, got %v", parts)
	}
}

func TestEntryToParts_NilEvent(t *testing.T) {
	state := NewStreamState()
	parts := EntryToParts(nil, state)
	if parts != nil {
		t.Errorf("expected nil, got %v", parts)
	}
}

func TestEntryToParts_ErrorEvent(t *testing.T) {
	state := NewStreamState()
	ev := &session.MessageEntry{
		ErrorMessage: "model overloaded",
	}
	parts := EntryToParts(ev, state)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	errPart, ok := parts[0].(Error)
	if !ok {
		t.Fatalf("expected Error, got %T", parts[0])
	}
	if errPart.ErrorText != "model overloaded" {
		t.Errorf("errorText = %q, want 'model overloaded'", errPart.ErrorText)
	}
}
