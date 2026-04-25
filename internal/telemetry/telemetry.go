// Package telemetry implements telemetry for ADK.
//
// WARNING: telemetry provided by ADK (internal/telemetry package) may change (e.g. attributes and their names)
// because we're in process to standardize and unify telemetry across all ADKs.
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.36.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/nickqiaoo/tadk/internal/version"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
)

const (
	systemName = "gcp.vertex.agent"

	executeToolName = "execute_tool"
	mergeToolName   = "(merged tools)"
)

var (
	gcpVertexAgentToolCallArgsName = attribute.Key("gcp.vertex.agent.tool_call_args")
	gcpVertexAgentEventID          = attribute.Key("gcp.vertex.agent.event_id")
	gcpVertexAgentToolResponseName = attribute.Key("gcp.vertex.agent.tool_response")
	gcpVertexAgentInvocationID     = attribute.Key("gcp.vertex.agent.invocation_id")
)

// tracer is the tracer instance for ADK go.
var tracer trace.Tracer = otel.GetTracerProvider().Tracer(
	systemName,
	trace.WithInstrumentationVersion(version.Version),
	trace.WithSchemaURL(semconv.SchemaURL),
)

type agent interface {
	Name() string
	Description() string
}

// StartInvokeAgentSpan starts a new semconv invoke_agent span.
// It returns a new context with the span and the span itself.
func StartInvokeAgentSpan(ctx context.Context, agent agent, sessionID, invocationID string) (context.Context, trace.Span) {
	agentName := agent.Name()
	spanCtx, span := tracer.Start(ctx, fmt.Sprintf("invoke_agent %s", agentName), trace.WithAttributes(
		gcpVertexAgentInvocationID.String(invocationID), // used by adk-web
		semconv.GenAIOperationNameInvokeAgent,
		semconv.GenAIAgentDescription(agent.Description()),
		semconv.GenAIAgentName(agentName),
		semconv.GenAIConversationID(sessionID),
	))

	return spanCtx, span
}

type TraceAgentResultParams struct {
	Error error
}

// TraceAgentResult records the result of the agent invocation, including status and error.
func TraceAgentResult(span trace.Span, params TraceAgentResultParams) {
	recordErrorAndStatus(span, params.Error)
}

// StartGenerateContentSpanParams contains parameters for [StartGenerateContentSpan].
type StartGenerateContentSpanParams struct {
	// ModelName is the name of the model being used for generation.
	ModelName string
	// InvocationID is the ID of the invocation.
	InvocationID string
}

// StartGenerateContentSpan starts a new semconv generate_content span.
func StartGenerateContentSpan(ctx context.Context, params StartGenerateContentSpanParams) (context.Context, trace.Span) {
	modelName := params.ModelName
	spanCtx, span := tracer.Start(ctx, fmt.Sprintf("generate_content %s", modelName), trace.WithAttributes(
		// Used by adk-web, can be removed once it reads the invocation id from invoke_agent span.
		gcpVertexAgentInvocationID.String(params.InvocationID),
		semconv.GenAIOperationNameGenerateContent,
		semconv.GenAIRequestModel(modelName),
	))
	return spanCtx, span
}

type TraceGenerateContentResultParams struct {
	Message *message.Message
	EventID  string
	Error    error
}

// TraceGenerateContentResult records the result of the generate_content operation, including token usage and finish reason.
func TraceGenerateContentResult(span trace.Span, params TraceGenerateContentResultParams) {
	recordErrorAndStatus(span, params.Error)
	if params.Message == nil {
		return
	}
	var stopReason model.StopReason
	var usage *model.Usage
	stopReason = params.Message.StopReason
	usage = params.Message.Usage
	span.SetAttributes(
		gcpVertexAgentEventID.String(params.EventID),
		semconv.GenAIResponseFinishReasons(string(stopReason)),
	)
	if usage != nil {
		span.SetAttributes(
			semconv.GenAIUsageInputTokens(usage.InputTokens),
			semconv.GenAIUsageOutputTokens(usage.OutputTokens),
		)
	}
}

// StartExecuteToolSpanParams contains parameters for [StartExecuteToolSpan].
type StartExecuteToolSpanParams struct {
	// ToolName is the name of the tool being executed.
	ToolName string
	// Args is the arguments of the tool call.
	Args map[string]any
}

// StartExecuteToolSpan starts a new semconv execute_tool span.
func StartExecuteToolSpan(ctx context.Context, params StartExecuteToolSpanParams) (context.Context, trace.Span) {
	toolName := params.ToolName
	spanCtx, span := tracer.Start(ctx, fmt.Sprintf("execute_tool %s", toolName), trace.WithAttributes(
		semconv.GenAIOperationNameExecuteTool,
		semconv.GenAIToolName(toolName),
		gcpVertexAgentToolCallArgsName.String(safeSerialize(params.Args))))
	return spanCtx, span
}

type TraceToolResultParams struct {
	// ToolDescription is a brief description of the tool's purpose.
	Description   string
	ResponseEvent session.DurableEntry
	Error         error
}

// TraceToolResult records the tool execution events.
func TraceToolResult(span trace.Span, params TraceToolResultParams) {
	recordErrorAndStatus(span, params.Error)

	attributes := []attribute.KeyValue{
		semconv.GenAIOperationNameKey.String(executeToolName),
		semconv.GenAIToolDescriptionKey.String(params.Description),
	}

	toolCallID := "<not specified>"
	toolResponse := "<not specified>"

	if params.ResponseEvent != nil {
		if b := params.ResponseEvent.Base(); b != nil {
			attributes = append(attributes, gcpVertexAgentEventID.String(b.ID))
		}
		if msgEntry, ok := params.ResponseEvent.(*session.MessageEntry); ok && msgEntry.Message != nil {
			if results := msgEntry.Message.ToolResults(); len(results) > 0 {
				tr := results[0]
				if tr.CallID != "" {
					toolCallID = tr.CallID
				}
				if tr.Content != nil {
					toolResponse = safeSerialize(tr.Content)
				}
			}
		}
	}

	attributes = append(attributes, semconv.GenAIToolCallIDKey.String(toolCallID))
	attributes = append(attributes, gcpVertexAgentToolResponseName.String(toolResponse))

	span.SetAttributes(attributes...)
}

func recordErrorAndStatus(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// WrapYield wraps a yield function to add tracing of values returned by iterators. Read [iter.Seq2] for more information about yield.
// Limitations:
// * if yield is called multiple times, then the span will be finalized with the values from the last call.
//
// Parameters:
//
//	span: The OpenTelemetry span to be managed.
//	yield: The original yield function `func(T, error) bool`.
//	finalizeSpan: A function `func(trace.Span, T, error)` called just before the span is ended to record final attributes.
//
// Returns:
//
//	wrapped: A wrapped yield function with the same signature as the original.
//	endSpan: A function to be called via `defer` to ensure the span is finalized with capture data and ended.
func WrapYield[T any](span trace.Span, yield func(T, error) bool, finalizeSpan func(trace.Span, T, error)) (wrapped func(T, error) bool, endSpan func()) {
	var val T
	var err error
	wrapped = func(v T, e error) bool {
		val = v
		err = e
		return yield(v, e)
	}
	endSpan = func() {
		finalizeSpan(span, val, err)
		span.End()
	}
	return wrapped, endSpan
}

// StartTrace starts a new span with the given name.
func StartTrace(ctx context.Context, traceName string) (context.Context, trace.Span) {
	return tracer.Start(ctx, traceName)
}

// TraceMergedToolCallsResult records the result of the merged tool calls, including status and tool execution events.
func TraceMergedToolCallsResult(span trace.Span, fnResponseEvent session.DurableEntry, err error) {
	recordErrorAndStatus(span, err)
	attributes := []attribute.KeyValue{
		semconv.GenAIOperationNameKey.String(executeToolName),
		semconv.GenAIToolNameKey.String(mergeToolName),
		semconv.GenAIToolDescriptionKey.String(mergeToolName),
		gcpVertexAgentToolCallArgsName.String("N/A"),
		gcpVertexAgentToolResponseName.String(safeSerialize(fnResponseEvent)),
	}
	if fnResponseEvent != nil {
		if b := fnResponseEvent.Base(); b != nil {
			attributes = append(attributes, gcpVertexAgentEventID.String(b.ID))
		}
	}
	span.SetAttributes(attributes...)
}

func safeSerialize(obj any) string {
	dump, err := json.Marshal(obj)
	if err != nil {
		return "<not serializable>"
	}
	return string(dump)
}
