package telemetry

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

func TestLogRequest(t *testing.T) {
	type wantEvent struct {
		name string
		body any
	}
	tests := []struct {
		name                  string
		captureMessageContent bool
		req                   *model.Request
		wantEvents            []wantEvent
	}{
		{
			name:                  "RequestWithSystemAndUserMessages",
			captureMessageContent: true,
			req: &model.Request{
				SystemPrompt: "System instruction part 1\nSystem instruction part 2",
				Messages: []*message.Message{
					message.NewUserMessage("Previous user message part 1\nPrevious user message part 2"),
					message.NewAssistantMessage("Previous agent message part 1\nPrevious agent message part 2"),
					message.NewUserMessage("User message part 1\nUser message part 2"),
				},
			},
			wantEvents: []wantEvent{
				{
					name: "gen_ai.system.message",
					body: map[string]any{
						"content": "System instruction part 1\nSystem instruction part 2",
					},
				},
				{
					name: "gen_ai.user.message",
					body: map[string]any{
						"content": map[string]any{
							"role":    "user",
							"content": "Previous user message part 1\nPrevious user message part 2",
						},
					},
				},
				{
					name: "gen_ai.user.message",
					body: map[string]any{
						"content": map[string]any{
							"role": "assistant",
							"content": []any{
								map[string]any{"type": "text", "text": "Previous agent message part 1\nPrevious agent message part 2"},
							},
						},
					},
				},
				{
					name: "gen_ai.user.message",
					body: map[string]any{
						"content": map[string]any{
							"role":    "user",
							"content": "User message part 1\nUser message part 2",
						},
					},
				},
			},
		},
		{
			name:                  "RequestWithNilMessages",
			captureMessageContent: true,
			req: &model.Request{
				SystemPrompt: "",
				Messages:     nil,
			},
			wantEvents: []wantEvent{
				{
					name: "gen_ai.system.message",
					body: map[string]any{
						"content": nil,
					},
				},
			},
		},
		{
			name:                  "RequestWithEmptyMessages",
			captureMessageContent: true,
			req: &model.Request{
				Messages: []*message.Message{},
			},
			wantEvents: []wantEvent{
				{
					name: "gen_ai.system.message",
					body: map[string]any{
						"content": nil,
					},
				},
			},
		},
		{
			name:                  "ElidedRequest",
			captureMessageContent: false,
			req: &model.Request{
				SystemPrompt: "System instruction",
				Messages:     []*message.Message{message.NewUserMessage("Hello")},
			},
			wantEvents: []wantEvent{
				{
					name: "gen_ai.system.message",
					body: map[string]any{
						"content": "<elided>",
					},
				},
				{
					name: "gen_ai.user.message",
					body: map[string]any{
						"content": "<elided>",
					},
				},
			},
		},
		{
			name:                  "ElidedRequestWithNilMessages",
			captureMessageContent: false,
			req: &model.Request{
				Messages: nil,
			},
			wantEvents: []wantEvent{
				{
					name: "gen_ai.system.message",
					body: map[string]any{
						"content": "<elided>",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			exporter := setup(t, tc.captureMessageContent)

			LogRequest(ctx, tc.req)

			if len(exporter.records) != len(tc.wantEvents) {
				t.Fatalf("expected %d records, got %d", len(tc.wantEvents), len(exporter.records))
			}

			for i, want := range tc.wantEvents {
				gotRecord := exporter.records[i]
				if gotRecord.EventName() != want.name {
					t.Errorf("record[%d]: expected event %q, got %q", i, want.name, gotRecord.EventName())
				}
				gotBody := toGoValue(gotRecord.Body())

				if diff := cmp.Diff(want.body, gotBody); diff != "" {
					t.Errorf("record[%d] body mismatch (-want +got):\n%s", i, diff)
				}
			}
		})
	}
}

func TestLogResponse(t *testing.T) {
	tests := []struct {
		name                  string
		msg                   *message.Message
		captureMessageContent bool
		wantName              string
		wantBody              map[string]any
	}{
		{
			name:                  "Response",
			captureMessageContent: true,
			msg: &message.Message{
				Role:       message.RoleAssistant,
				Content:    []message.Content{&message.TextContent{Text: "Text part 1\nText part 2"}},
				StopReason: message.StopReasonStop,
			},
			wantName: "gen_ai.choice",
			wantBody: map[string]any{
				"index":         int64(0),
				"finish_reason": "stop",
				"content": map[string]any{
					"role":       "assistant",
					"stopReason": "stop",
					"content": []any{
						map[string]any{"type": "text", "text": "Text part 1\nText part 2"},
					},
				},
			},
		},
		{
			name:                  "ResponseWithToolCall",
			captureMessageContent: true,
			msg: &message.Message{
				Role:       message.RoleAssistant,
				StopReason: message.StopReasonToolUse,
				Content: []message.Content{
					&message.ThinkingContent{Thinking: "Call tools"},
					&message.ToolCallContent{Name: "myTool1", ID: "id1", Arguments: map[string]any{"arg1": "val1"}},
					&message.ToolCallContent{Name: "myTool2", ID: "id2", Arguments: map[string]any{"arg2": "val2"}},
				},
			},
			wantName: "gen_ai.choice",
			wantBody: map[string]any{
				"index":         int64(0),
				"finish_reason": "toolUse",
				"content": map[string]any{
					"role":       "assistant",
					"stopReason": "toolUse",
					"content": []any{
						map[string]any{
							"type":     "thinking",
							"thinking": "Call tools",
						},
						map[string]any{
							"type":      "toolCall",
							"name":      "myTool1",
							"id":        "id1",
							"arguments": map[string]any{"arg1": "val1"},
						},
						map[string]any{
							"type":      "toolCall",
							"name":      "myTool2",
							"id":        "id2",
							"arguments": map[string]any{"arg2": "val2"},
						},
					},
				},
			},
		},
		{
			name:                  "NilResponse",
			captureMessageContent: true,
			msg:                   nil,
			wantName:              "gen_ai.choice",
			wantBody: map[string]any{
				"index":   int64(0),
				"content": nil,
			},
		},
		{
			name:                  "ElidedResponse",
			captureMessageContent: false,
			msg: &message.Message{
				Role:       message.RoleAssistant,
				Content:    []message.Content{&message.TextContent{Text: "Response part 1\nResponse part 2"}},
				StopReason: message.StopReasonStop,
			},
			wantName: "gen_ai.choice",
			wantBody: map[string]any{
				"index":         int64(0),
				"finish_reason": "stop",
				"content":       "<elided>",
			},
		},
		{
			name:                  "ElidedNilResponse",
			captureMessageContent: false,
			msg:                   nil,
			wantName:              "gen_ai.choice",
			wantBody: map[string]any{
				"index":   int64(0),
				"content": "<elided>",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exporter := setup(t, tc.captureMessageContent)

			LogResponse(t.Context(), tc.msg)

			if len(exporter.records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(exporter.records))
			}
			record := exporter.records[0]
			if record.EventName() != tc.wantName {
				t.Errorf("expected event %q, got %q", tc.wantName, record.EventName())
			}

			got := toGoValue(record.Body())
			if diff := cmp.Diff(tc.wantBody, got); diff != "" {
				t.Errorf("Body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSpanIDPropagation(t *testing.T) {
	ctx, span := otel.Tracer("test").Start(context.Background(), "test")
	defer span.End()

	exporter := setup(t, false)

	req := &model.Request{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []*message.Message{message.NewUserMessage("Hello")},
	}

	LogRequest(ctx, req)
	LogResponse(ctx, &message.Message{})

	if len(exporter.records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(exporter.records))
	}

	wantSpanID := span.SpanContext().SpanID()
	for _, record := range exporter.records {
		if got := record.SpanID(); got != wantSpanID {
			t.Errorf("expected span ID %q, got %q", wantSpanID, got)
		}
	}
}

func setup(t *testing.T, elided bool) *inMemoryExporter {
	exporter := &inMemoryExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	originalLogger := otelLogger
	otelLogger = provider.Logger("test")
	t.Cleanup(func() {
		otelLogger = originalLogger
	})

	original := getGenAICaptureMessageContent()
	SetGenAICaptureMessageContent(elided)
	t.Cleanup(func() {
		SetGenAICaptureMessageContent(original)
	})
	return exporter
}

type inMemoryExporter struct {
	records []sdklog.Record
}

func (e *inMemoryExporter) Export(ctx context.Context, records []sdklog.Record) error {
	e.records = append(e.records, records...)
	return nil
}

func (e *inMemoryExporter) Shutdown(ctx context.Context) error   { return nil }
func (e *inMemoryExporter) ForceFlush(ctx context.Context) error { return nil }

// toGoValue converts a log.Value to a Go value for easier testing.
func toGoValue(v log.Value) any {
	switch v.Kind() {
	case log.KindBool:
		return v.AsBool()
	case log.KindFloat64:
		return v.AsFloat64()
	case log.KindInt64:
		return v.AsInt64()
	case log.KindString:
		return v.AsString()
	case log.KindBytes:
		return v.AsBytes()
	case log.KindSlice:
		var s []any
		for _, v := range v.AsSlice() {
			s = append(s, toGoValue(v))
		}
		return s
	case log.KindMap:
		m := make(map[string]any)
		for _, kv := range v.AsMap() {
			m[kv.Key] = toGoValue(kv.Value)
		}
		return m
	default:
		return nil
	}
}
