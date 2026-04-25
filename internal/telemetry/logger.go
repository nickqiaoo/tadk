package telemetry

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	semconv "go.opentelemetry.io/otel/semconv/v1.36.0"

	"github.com/nickqiaoo/tadk/internal/version"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// genAICaptureMessageContent is true if message content should be elided. False by default.
var genAICaptureMessageContent atomic.Bool

// SetGenAICaptureMessageContent sets whether message content should be elided.
func SetGenAICaptureMessageContent(capture bool) {
	genAICaptureMessageContent.Store(capture)
}

// getGenAICaptureMessageContent returns whether message content should be elided.
func getGenAICaptureMessageContent() bool {
	return genAICaptureMessageContent.Load()
}

const elidedContent = "<elided>"

var otelLogger = global.GetLoggerProvider().Logger(
	systemName,
	log.WithSchemaURL(semconv.SchemaURL),
	log.WithInstrumentationVersion(version.Version),
)

// LogRequest logs the request to the model - the system message and user messages.
func LogRequest(ctx context.Context, req *model.Request) {
	logSystemMessage(ctx, req)
	for _, msg := range req.Messages {
		logUserMessage(ctx, msg)
	}
}

// LogResponse logs the inference result.
func LogResponse(ctx context.Context, msg *message.Message) {
	record := log.Record{}
	record.SetEventName("gen_ai.choice")

	var stopReason string
	if msg != nil {
		stopReason = string(msg.StopReason)
	}

	kvs := []log.KeyValue{
		log.Int("index", 0),
		{Key: "content", Value: messageToLogValue(msg)},
	}

	if stopReason != "" {
		kvs = append(kvs, log.String("finish_reason", stopReason))
	}
	record.SetBody(log.MapValue(kvs...))
	otelLogger.Emit(ctx, record)
}

// logSystemMessage logs the system message from the request.
func logSystemMessage(ctx context.Context, req *model.Request) {
	record := log.Record{}
	record.SetEventName("gen_ai.system.message")
	record.SetBody(log.MapValue(
		log.KeyValue{Key: "content", Value: extractSystemMessage(req)},
	))
	otelLogger.Emit(ctx, record)
}

// logUserMessage logs a user message from the request.
func logUserMessage(ctx context.Context, msg *message.Message) {
	record := log.Record{}
	record.SetEventName("gen_ai.user.message")
	record.SetBody(log.MapValue(
		log.KeyValue{Key: "content", Value: messageToLogValue(msg)},
	))
	otelLogger.Emit(ctx, record)
}

// extractSystemMessage extracts the system prompt from the request.
func extractSystemMessage(req *model.Request) log.Value {
	if !getGenAICaptureMessageContent() {
		return log.StringValue(elidedContent)
	}
	if req == nil || req.SystemPrompt == "" {
		return log.Value{}
	}
	return log.StringValue(req.SystemPrompt)
}

func messageToLogValue(msg *message.Message) log.Value {
	return toLogValue(messageToJSONLikeValue(msg))
}

// messageToJSONLikeValue converts a message.Message to a JSON-like value for logging.
func messageToJSONLikeValue(msg *message.Message) any {
	if !getGenAICaptureMessageContent() {
		return elidedContent
	}
	if msg == nil {
		return nil
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return "<not_serializable>"
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "<not_serializable>"
	}
	return m
}
