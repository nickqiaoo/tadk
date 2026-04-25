package model

import (
	"context"
	"iter"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
)

// ModelAdapter is the provider-agnostic interface for LLM interactions.
// Each provider (OpenAI, Anthropic, Gemini, etc.) implements this interface,
// converting between the canonical message.Message format and their native API format internally.
type ModelAdapter interface {
	// Name returns an optional adapter label for diagnostics.
	Name() string

	// Generate sends a request to the LLM and returns the final assistant message.
	Generate(ctx context.Context, req *Request) (*message.Message, error)

	// Stream sends a request to the LLM and returns a canonical event stream
	// plus access to the final response once streaming completes.
	Stream(ctx context.Context, req *Request) *EventStream
}

// EventStream carries streaming events and exposes the final result.
type EventStream struct {
	Events iter.Seq2[event.Event, error]
	Result func() (*message.Message, error)
}

// NewEventStream creates an EventStream from an event iterator and a result function.
func NewEventStream(events iter.Seq2[event.Event, error], result func() (*message.Message, error)) *EventStream {
	return &EventStream{Events: events, Result: result}
}

// Request is the provider-agnostic LLM request.
type Request struct {
	// Model is the model identifier to use.
	Model string

	// SystemPrompt is the system-level instruction for the model.
	SystemPrompt string

	// Messages is the conversation history in canonical format.
	Messages []*message.Message

	// Tools is the list of tools available to the model.
	Tools []ToolDefinition

	// Config holds generation parameters. Read-only: adapters and framework
	// code must not mutate the pointee. Callers wishing to override settings
	// should construct a new GenerateConfig (see CloneGenerateConfig).
	Config *GenerateConfig
}

type CacheRetention string

const (
	CacheRetentionNone  CacheRetention = "none"
	CacheRetentionShort CacheRetention = "short"
	CacheRetentionLong  CacheRetention = "long"
)

type Transport string

const (
	TransportSSE       Transport = "sse"
	TransportWebSocket Transport = "websocket"
	TransportAuto      Transport = "auto"
)

type ThinkingBudgets struct {
	Minimal int
	Low     int
	Medium  int
	High    int
	XHigh   int
}

type ToolChoiceMode string

const (
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceTool     ToolChoiceMode = "tool"
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	// Name is the tool's identifier.
	Name string

	// Description explains what the tool does.
	Description string

	// Parameters is the JSON Schema for the tool's input parameters.
	Parameters map[string]any
}

// GenerateConfig holds provider-agnostic generation parameters.
type GenerateConfig struct {
	Temperature     float64
	TopP            float64
	TopK            int
	MaxOutputTokens int
	StopSequences   []string
	Seed            int

	// ThinkingLevel controls the model's reasoning depth (if supported).
	// Values: "off", "minimal", "low", "medium", "high".
	ThinkingLevel   string
	ThinkingBudgets *ThinkingBudgets

	// ResponseMIMEType constrains the output format (e.g., "application/json").
	ResponseMIMEType string

	// ResponseSchema constrains the output to match a JSON Schema (if supported).
	ResponseSchema map[string]any

	Headers         map[string]string
	Metadata        map[string]any
	SessionID       string
	Transport       Transport
	CacheRetention  CacheRetention
	MaxRetryDelayMs int

	ToolChoice        *ToolChoice
	ParallelToolCalls bool
	Store             bool
	ServiceTier       string
	CandidateCount    int
	PromptCacheKey    string
	SafetyIdentifier  string

	ProviderOptions map[string]any
}

type Usage = message.Usage
type StopReason = message.StopReason

const (
	StopReasonStop    = message.StopReasonStop
	StopReasonLength  = message.StopReasonLength
	StopReasonToolUse = message.StopReasonToolUse
	StopReasonError   = message.StopReasonError
	StopReasonAborted = message.StopReasonAborted
	StopReasonSafety  = message.StopReasonSafety
	StopReasonUnknown = message.StopReasonUnknown
)
