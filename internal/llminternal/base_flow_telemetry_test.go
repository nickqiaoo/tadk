package llminternal

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.36.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/telemetry"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

type mockModelForTest struct {
	name     string
	generate func(ctx context.Context, req *model.Request) (*message.Message, error)
	stream   func(ctx context.Context, req *model.Request) *model.EventStream
}

func (m *mockModelForTest) Name() string {
	return m.name
}

func (m *mockModelForTest) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	if m.generate != nil {
		return m.generate(ctx, req)
	}
	if m.stream != nil {
		return m.stream(ctx, req).Result()
	}
	return nil, nil
}

func (m *mockModelForTest) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	if m.stream != nil {
		return m.stream(ctx, req)
	}
	msg, err := m.Generate(ctx, req)
	builder := model.NewAssistantEventBuilder()
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			if err != nil {
				yield(nil, err)
				return
			}
			if msg == nil {
				return
			}
			if text := msg.Text(); text != "" {
				for _, ev := range builder.TextDelta(text) {
					if !yield(ev, nil) {
						return
					}
				}
				for _, ev := range builder.FinalizeTextThinking() {
					if !yield(ev, nil) {
						return
					}
				}
			}
			yield(builder.Done(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), err },
	)
}

func generateContent(ctx agent.InvocationContext, m model.ModelAdapter, req *model.Request, useStream bool) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		spanCtx, span := telemetry.StartGenerateContentSpan(ctx.Context(), telemetry.StartGenerateContentSpanParams{
			ModelName:    m.Name(),
			InvocationID: ctx.InvocationID(),
		})
		ctx = ctx.WithContext(spanCtx)
		telemetry.LogRequest(ctx.Context(), req)

		var lastMsg *message.Message
		var lastErr error
		spanEnded := false
		endSpanAndTrackResult := func() {
			if spanEnded {
				return
			}
			var eventID string
			if lastMsg != nil {
				eventID = uuid.New().String()
			}
			telemetry.TraceGenerateContentResult(span, telemetry.TraceGenerateContentResultParams{
				Message: lastMsg,
				EventID: eventID,
				Error:   lastErr,
			})
			span.End()
			spanEnded = true
		}
		defer endSpanAndTrackResult()

		if useStream {
			stream := m.Stream(ctx.Context(), req)
			for ev, err := range stream.Events {
				if err != nil {
					lastErr = err
					endSpanAndTrackResult()
					if !yield(nil, err) {
						return
					}
					return
				}
				if !yield(ev, nil) {
					return
				}
			}
			if final, err := stream.Result(); err != nil {
				lastErr = err
				endSpanAndTrackResult()
				yield(nil, err)
				return
			} else if final != nil {
				lastMsg = final
				telemetry.LogResponse(ctx.Context(), lastMsg)
				endSpanAndTrackResult()
			}
			return
		}

		resp, err := m.Generate(ctx.Context(), req)
		lastMsg = resp
		lastErr = err
		if err != nil {
			endSpanAndTrackResult()
		} else if resp != nil {
			telemetry.LogResponse(ctx.Context(), resp)
			endSpanAndTrackResult()
		}
		if !yield(event.NewMessageEnd(resp), err) {
			return
		}
	}
}

var (
	testExporter *tracetest.InMemoryExporter
	initTracer   sync.Once
)

func TestGenerateContentTracing(t *testing.T) {
	setupTestTracer(t)

	modelMock := &mockModelForTest{
		name: "test-model",
		stream: func(ctx context.Context, req *model.Request) *model.EventStream {
			builder := model.NewAssistantEventBuilder()
			final := &message.Message{
				Role: message.RoleAssistant,
				Usage: &message.Usage{
					InputTokens:  10,
					OutputTokens: 20,
				},
			}
			return model.NewEventStream(func(yield func(event.Event, error) bool) {
				for _, ev := range builder.TextDelta("partial") {
					if !yield(ev, nil) {
						return
					}
				}
				// Verify span NOT ended.
				gotSpans := testExporter.GetSpans()
				if len(gotSpans) != 0 {
					t.Errorf("expected 0 spans after partial response, got %d", len(gotSpans))
				}
				for _, ev := range builder.FinalizeTextThinking() {
					if !yield(ev, nil) {
						return
					}
				}
				if !yield(builder.Done(final), nil) {
					return
				}
				// Verify span is still open during streaming; it closes after Result().
				gotSpans = testExporter.GetSpans()
				if len(gotSpans) != 0 {
					t.Errorf("expected 0 spans before stream result, got %d", len(gotSpans))
				}
			}, func() (*message.Message, error) { return message.Clone(final), nil })
		},
	}

	ctx := agent.NewInvocationContext(context.Background(), agent.InvocationContextParams{})

	for range generateContent(ctx, modelMock, &model.Request{}, true) {
	}

	// Verify that there is only single span.
	gotSpans := testExporter.GetSpans()
	if len(gotSpans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(gotSpans))
	}
	gotSpan := gotSpans[0]

	if gotSpan.Name != "generate_content test-model" {
		t.Errorf("expected span name %q, got %q", "generate_content test-model", gotSpan.Name)
	}

	// Verify span attributes.
	attrs := make(map[attribute.Key]string)
	for _, kv := range gotSpan.Attributes {
		attrs[kv.Key] = kv.Value.Emit()
	}

	if val := attrs[semconv.GenAIUsageInputTokensKey]; val != "10" {
		t.Errorf("expected input tokens 10, got %s", val)
	}
	if val := attrs[semconv.GenAIUsageOutputTokensKey]; val != "20" {
		t.Errorf("expected output tokens 20, got %s", val)
	}
	if val := attrs["gcp.vertexai.invocation_id"]; val != "" {
		t.Errorf("expected invocation id, got %s", val)
	}
}

func TestGenerateContentTracingNoFinalResponse(t *testing.T) {
	setupTestTracer(t)

	modelMock := &mockModelForTest{
		name: "test-model",
		stream: func(ctx context.Context, req *model.Request) *model.EventStream {
			builder := model.NewAssistantEventBuilder()
			final := &message.Message{
				Role: message.RoleAssistant,
				Usage: &message.Usage{
					InputTokens:  10,
					OutputTokens: 20,
				},
			}
			return model.NewEventStream(func(yield func(event.Event, error) bool) {
				for _, ev := range builder.TextDelta("partial") {
					if !yield(ev, nil) {
						return
					}
				}
				// Verify span NOT ended.
				gotSpans := testExporter.GetSpans()
				if len(gotSpans) != 0 {
					t.Errorf("expected 0 spans after partial response, got %d", len(gotSpans))
				}
			}, func() (*message.Message, error) { return message.Clone(final), nil })
		},
	}

	ctx := agent.NewInvocationContext(context.Background(), agent.InvocationContextParams{})

	for range generateContent(ctx, modelMock, &model.Request{}, true) {
	}

	// Verify that there is only single span.
	gotSpans := testExporter.GetSpans()
	if len(gotSpans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(gotSpans))
	}
	gotSpan := gotSpans[0]

	if gotSpan.Name != "generate_content test-model" {
		t.Errorf("expected span name %q, got %q", "generate_content test-model", gotSpan.Name)
	}

	// Verify span attributes.
	attrs := make(map[attribute.Key]string)
	for _, kv := range gotSpan.Attributes {
		attrs[kv.Key] = kv.Value.Emit()
	}

	if val := attrs[semconv.GenAIUsageInputTokensKey]; val != "10" {
		t.Errorf("expected input tokens 10, got %s", val)
	}
	if val := attrs[semconv.GenAIUsageOutputTokensKey]; val != "20" {
		t.Errorf("expected output tokens 20, got %s", val)
	}
}

func TestGenerateContentTracingError(t *testing.T) {
	setupTestTracer(t)

	modelMock := &mockModelForTest{
		name: "test-model",
		stream: func(ctx context.Context, req *model.Request) *model.EventStream {
			builder := model.NewAssistantEventBuilder()
			return model.NewEventStream(func(yield func(event.Event, error) bool) {
				for _, ev := range builder.TextDelta("partial") {
					if !yield(ev, nil) {
						return
					}
				}
				// Yield error.
				yield(nil, errors.New("test error"))
				// Verify span ended.
				gotSpans := testExporter.GetSpans()
				if len(gotSpans) != 1 {
					t.Errorf("expected 1 span after error, got %d", len(gotSpans))
				}
			}, func() (*message.Message, error) { return nil, errors.New("test error") })
		},
	}

	ctx := agent.NewInvocationContext(context.Background(), agent.InvocationContextParams{})

	for range generateContent(ctx, modelMock, &model.Request{}, true) {
	}

	// Verify that there is only single span.
	gotSpans := testExporter.GetSpans()
	if len(gotSpans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(gotSpans))
	}
	gotSpan := gotSpans[0]

	if gotSpan.Name != "generate_content test-model" {
		t.Errorf("expected span name %q, got %q", "generate_content test-model", gotSpan.Name)
	}

	if gotSpan.Status.Code != codes.Error {
		t.Errorf("expected span status %q, got %q", codes.Error, gotSpan.Status.Code)
	}

	if gotSpan.Status.Description != "test error" {
		t.Errorf("expected span status description %q, got %q", "test error", gotSpan.Status.Description)
	}
}

func setupTestTracer(t *testing.T) {
	t.Helper()
	initTracer.Do(func() {
		// internal/telemetry initializes the global tracer provider once at startup.
		// Subsequent calls to otel.SetTracerProvider don't update existing tracer providers, so we can override only once.
		testExporter = tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(testExporter),
		)
		otel.SetTracerProvider(tp)
	})
	// Reset the exporter before each test to avoid flakiness.
	testExporter.Reset()
	t.Cleanup(func() {
		testExporter.Reset()
	})
}

type inMemoryLogExporter struct {
	records []sdklog.Record
}

func (e *inMemoryLogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	e.records = append(e.records, records...)
	return nil
}
func (e *inMemoryLogExporter) Shutdown(ctx context.Context) error   { return nil }
func (e *inMemoryLogExporter) ForceFlush(ctx context.Context) error { return nil }

func TestLoggingSpanIDPropagation(t *testing.T) {
	setupTestTracer(t)
	logExporter := setupLoggerProvider(t)

	var wantSpanID trace.SpanID
	modelMock := &mockModelForTest{
		name: "test-model",
		stream: func(ctx context.Context, req *model.Request) *model.EventStream {
			// Capture the span ID.
			wantSpanID = trace.SpanFromContext(ctx).SpanContext().SpanID()
			if !wantSpanID.IsValid() {
				t.Fatalf("expected span ID to be valid, got %q", wantSpanID)
			}
			builder := model.NewAssistantEventBuilder()
			final := &message.Message{
				Role:    message.RoleAssistant,
				Content: []message.Content{&message.TextContent{Text: "Response"}},
				Usage:   &message.Usage{InputTokens: 1, OutputTokens: 2},
			}
			return model.NewEventStream(func(yield func(event.Event, error) bool) {
				for _, ev := range builder.TextDelta("Response") {
					if !yield(ev, nil) {
						return
					}
				}
				for _, ev := range builder.FinalizeTextThinking() {
					if !yield(ev, nil) {
						return
					}
				}
				yield(builder.Done(final), nil)
			}, func() (*message.Message, error) { return message.Clone(final), nil })
		},
	}

	req := &model.Request{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []*message.Message{message.NewUserMessage("Hello")},
	}

	ctx := agent.NewInvocationContext(context.Background(), agent.InvocationContextParams{})
	for range generateContent(ctx, modelMock, req, true) {
	}

	if len(logExporter.records) != 3 {
		t.Fatalf("expected 3 log records, got %d", len(logExporter.records))
	}

	wantEvents := []string{
		"gen_ai.system.message",
		"gen_ai.user.message",
		"gen_ai.choice",
	}

	for i, record := range logExporter.records {
		if got := record.SpanID(); got != wantSpanID {
			t.Errorf("record[%d]: expected span ID %q, got %q", i, wantSpanID, got)
		}
		if got := record.EventName(); got != wantEvents[i] {
			t.Errorf("record[%d]: expected event name %q, got %q", i, wantEvents[i], got)
		}
	}
}

func setupLoggerProvider(t *testing.T) *inMemoryLogExporter {
	logExporter := &inMemoryLogExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExporter)),
	)
	originalProvider := global.GetLoggerProvider()
	global.SetLoggerProvider(provider)
	t.Cleanup(func() {
		global.SetLoggerProvider(originalProvider)
	})
	return logExporter
}
