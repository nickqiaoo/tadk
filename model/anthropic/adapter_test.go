package anthropic

import (
	"encoding/json"
	"testing"

	anthropicapi "github.com/anthropics/anthropic-sdk-go"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

func TestConvertAccumulated_ReconstructsFinalStreamResult(t *testing.T) {
	t.Parallel()

	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","content":[],"model":"claude-sonnet-4-20250514","role":"assistant","stop_reason":"","stop_sequence":"","type":"message","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":3,"cache_creation_input_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"Let me","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" think"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":"Hello"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"ad"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"k\"}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":"","container":{"expires_at":"2026-01-01T00:00:00Z","id":"container_1","type":"container"}},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	}

	var acc anthropicapi.Message
	for _, raw := range events {
		var event anthropicapi.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
		}
		if err := acc.Accumulate(event); err != nil {
			t.Fatalf("acc.Accumulate(%s) error = %v", raw, err)
		}
	}

	a := &adapter{}
	got := a.convertAccumulated(&acc, "claude-sonnet-4-20250514")

	if got == nil {
		t.Fatalf("convertAccumulated() returned nil message")
	}
	if got.StopReason != model.StopReasonToolUse {
		t.Fatalf("StopReason = %q, want %q", got.StopReason, model.StopReasonToolUse)
	}
	if got.Usage == nil {
		t.Fatalf("Usage = nil, want non-nil")
	}
	if got.Usage.InputTokens != 12 || got.Usage.OutputTokens != 7 || got.Usage.CacheReadTokens != 3 || got.Usage.TotalTokens != 19 {
		t.Fatalf("Usage = %+v, want input=12 output=7 cacheRead=3 total=19", *got.Usage)
	}

	content := got.Content
	if len(content) != 3 {
		t.Fatalf("len(content) = %d, want 3", len(content))
	}

	thinking := content[0].(*message.ThinkingContent)
	if thinking.Thinking != "Let me think" || thinking.ThinkingSignature != "sig-1" {
		t.Fatalf("thinking = %+v, want text/signature reconstructed", thinking)
	}

	text := content[1].(*message.TextContent)
	if text.Text != "Hello world" {
		t.Fatalf("text = %q, want %q", text.Text, "Hello world")
	}

	toolCall := content[2].(*message.ToolCallContent)
	if toolCall.ID != "toolu_1" || toolCall.Name != "search" {
		t.Fatalf("toolCall = %+v, want id/name reconstructed", toolCall)
	}
	if q, ok := toolCall.Arguments["q"].(string); !ok || q != "adk" {
		t.Fatalf("toolCall.Arguments = %#v, want q=adk", toolCall.Arguments)
	}
}

func TestStreamEvents_EmitsToolCallStartAndDelta(t *testing.T) {
	t.Parallel()

	var acc anthropicapi.Message
	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","content":[],"model":"claude-sonnet-4-20250514","role":"assistant","stop_reason":"","stop_sequence":"","type":"message","usage":{"input_tokens":1,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"adk\"}"}}`,
	}

	a := &adapter{}
	builder := model.NewAssistantEventBuilder()
	var gotStart *event.ToolCallDelta
	var gotDelta *event.ToolCallDelta
	for _, raw := range events {
		var apiEvent anthropicapi.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &apiEvent); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
		}
		if err := acc.Accumulate(apiEvent); err != nil {
			t.Fatalf("acc.Accumulate(%s) error = %v", raw, err)
		}
		for _, out := range a.streamEvents(builder, &apiEvent, &acc) {
			switch ev := out.(type) {
			case *event.ToolCallDelta:
				if gotStart == nil && ev.Delta == "" {
					gotStart = ev
				} else if ev.Delta != "" {
					gotDelta = ev
				}
			}
		}
	}

	if gotStart == nil {
		t.Fatal("expected toolcall_start event")
	}
	if gotDelta == nil {
		t.Fatal("expected toolcall_delta event")
	}
	if gotDelta.Partial == nil {
		t.Fatal("expected partial assistant message")
	}
	calls := gotDelta.Partial.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("ToolCalls() = %d, want 1", len(calls))
	}
	tc := calls[0]
	if tc.ID != "toolu_1" || tc.Name != "search" {
		t.Fatalf("tool call = %+v, want id/name reconstructed", tc)
	}
	if q, ok := tc.Args["q"].(string); !ok || q != "adk" {
		t.Fatalf("tool call args = %#v, want q=adk", tc.Args)
	}
}
