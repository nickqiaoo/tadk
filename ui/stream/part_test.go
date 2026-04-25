package stream

import (
	"encoding/json"
	"strings"
	"testing"
)

// mustParse strips SSE envelope and parses JSON payload.
func mustParse(t *testing.T, formatted string) map[string]any {
	t.Helper()
	s := strings.TrimPrefix(formatted, "data: ")
	s = strings.TrimSpace(s)
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("failed to parse JSON %q: %v", s, err)
	}
	return m
}

func assertSSE(t *testing.T, formatted string) {
	t.Helper()
	if !strings.HasPrefix(formatted, "data: ") {
		t.Errorf("should start with 'data: ', got %q", formatted)
	}
	if !strings.HasSuffix(formatted, "\n\n") {
		t.Errorf("should end with '\\n\\n', got %q", formatted)
	}
}

func TestStartFormat(t *testing.T) {
	p := Start{MessageID: "msg_123"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	assertSSE(t, got)
	m := mustParse(t, got)
	if m["type"] != "start" {
		t.Errorf("type = %q, want 'start'", m["type"])
	}
	if m["messageId"] != "msg_123" {
		t.Errorf("messageId = %q, want 'msg_123'", m["messageId"])
	}
}

func TestFinishFormat(t *testing.T) {
	p := Finish{FinishReason: FinishReasonStop}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	assertSSE(t, got)
	m := mustParse(t, got)
	if m["type"] != "finish" {
		t.Errorf("type = %q, want 'finish'", m["type"])
	}
	if m["finishReason"] != "stop" {
		t.Errorf("finishReason = %q, want 'stop'", m["finishReason"])
	}
}

func TestStartStepFormat(t *testing.T) {
	got, err := StartStep{}.Format()
	if err != nil {
		t.Fatal(err)
	}
	want := "data: {\"type\":\"start-step\"}\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFinishStepFormat(t *testing.T) {
	got, err := FinishStep{}.Format()
	if err != nil {
		t.Fatal(err)
	}
	want := "data: {\"type\":\"finish-step\"}\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextDeltaFormat(t *testing.T) {
	p := TextDelta{ID: "text_0", Delta: "Hello, world!"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	assertSSE(t, got)
	m := mustParse(t, got)
	if m["type"] != "text-delta" {
		t.Errorf("type = %q, want 'text-delta'", m["type"])
	}
	if m["id"] != "text_0" {
		t.Errorf("id = %q, want 'text_0'", m["id"])
	}
	if m["delta"] != "Hello, world!" {
		t.Errorf("delta = %q, want 'Hello, world!'", m["delta"])
	}
}

func TestReasoningDeltaFormat(t *testing.T) {
	p := ReasoningDelta{ID: "reasoning_0", Delta: "Let me think..."}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "reasoning-delta" {
		t.Errorf("type = %q, want 'reasoning-delta'", m["type"])
	}
	if m["delta"] != "Let me think..." {
		t.Errorf("delta = %q, want 'Let me think...'", m["delta"])
	}
}

func TestToolInputAvailableFormat(t *testing.T) {
	p := ToolInputAvailable{
		ToolCallID: "call_1",
		ToolName:   "bash",
		Input:      map[string]any{"command": "ls -la"},
	}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	assertSSE(t, got)
	m := mustParse(t, got)
	if m["type"] != "tool-input-available" {
		t.Errorf("type = %q, want 'tool-input-available'", m["type"])
	}
	if m["toolCallId"] != "call_1" {
		t.Errorf("toolCallId = %q, want 'call_1'", m["toolCallId"])
	}
	if m["toolName"] != "bash" {
		t.Errorf("toolName = %q, want 'bash'", m["toolName"])
	}
}

func TestToolOutputAvailableFormat(t *testing.T) {
	p := ToolOutputAvailable{
		ToolCallID: "call_1",
		Output:     map[string]any{"output": "file.txt"},
	}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "tool-output-available" {
		t.Errorf("type = %q, want 'tool-output-available'", m["type"])
	}
	if m["toolCallId"] != "call_1" {
		t.Errorf("toolCallId = %q, want 'call_1'", m["toolCallId"])
	}
}

func TestToolOutputErrorFormat(t *testing.T) {
	p := ToolOutputError{ToolCallID: "call_1", ErrorText: "command failed"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "tool-output-error" {
		t.Errorf("type = %q, want 'tool-output-error'", m["type"])
	}
	if m["errorText"] != "command failed" {
		t.Errorf("errorText = %q, want 'command failed'", m["errorText"])
	}
}

func TestErrorFormat(t *testing.T) {
	p := Error{ErrorText: "something went wrong"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "error" {
		t.Errorf("type = %q, want 'error'", m["type"])
	}
	if m["errorText"] != "something went wrong" {
		t.Errorf("errorText = %q, want 'something went wrong'", m["errorText"])
	}
}

func TestDataPartFormat(t *testing.T) {
	p := NewInterruptPart(map[string]any{"id": "int_1"})
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	assertSSE(t, got)
	m := mustParse(t, got)
	if m["type"] != "data-interrupt" {
		t.Errorf("type = %q, want 'data-interrupt'", m["type"])
	}
}

func TestToolInputStartFormat(t *testing.T) {
	p := ToolInputStart{ToolCallID: "call_1", ToolName: "read_file"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "tool-input-start" {
		t.Errorf("type = %q, want 'tool-input-start'", m["type"])
	}
	if m["toolCallId"] != "call_1" {
		t.Errorf("toolCallId = %q, want 'call_1'", m["toolCallId"])
	}
}

func TestAbortFormat(t *testing.T) {
	p := Abort{Reason: "user cancelled"}
	got, err := p.Format()
	if err != nil {
		t.Fatal(err)
	}
	m := mustParse(t, got)
	if m["type"] != "abort" {
		t.Errorf("type = %q, want 'abort'", m["type"])
	}
	if m["reason"] != "user cancelled" {
		t.Errorf("reason = %q, want 'user cancelled'", m["reason"])
	}
}
