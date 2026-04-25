// Package stream defines the UI message stream wire protocol.
//
// Parts are the atomic units of the output stream, compatible with the
// Vercel AI SDK UI Message Stream Protocol (v5+). Each Part serializes to
// SSE format "data: {json}\n\n" and can be consumed by useChat() on the frontend.
//
// This package is optional UI transport glue. SDK callers that want the raw
// agent runtime should use runner.Run, which returns event.Event values.
package stream

import (
	"encoding/json"
	"fmt"
)

// Part is a single chunk of the UI Message Stream.
// Each Part can be serialized to SSE wire format: "data: {json}\n\n"
type Part interface {
	// Format serializes the Part to SSE wire format.
	Format() (string, error)
}

// Done is the stream terminator.
const Done = "data: [DONE]\n\n"

// FinishReason defines why a message finished.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonContentFilter FinishReason = "content-filter"
	FinishReasonToolCalls     FinishReason = "tool-calls"
	FinishReasonError         FinishReason = "error"
	FinishReasonUnknown       FinishReason = "unknown"
)

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Start signals the beginning of a new message.
type Start struct {
	MessageID       string         `json:"messageId,omitempty"`
	MessageMetadata map[string]any `json:"messageMetadata,omitempty"`
}

func (p Start) Format() (string, error) { return formatSSE("start", p) }

// Finish signals the end of the message.
type Finish struct {
	FinishReason    FinishReason   `json:"finishReason,omitempty"`
	MessageMetadata map[string]any `json:"messageMetadata,omitempty"`
}

func (p Finish) Format() (string, error) { return formatSSE("finish", p) }

// Abort signals the stream was aborted.
type Abort struct {
	Reason string `json:"reason,omitempty"`
}

func (p Abort) Format() (string, error) { return formatSSE("abort", p) }

// StartStep signals the beginning of a step (one LLM call).
type StartStep struct{}

func (p StartStep) Format() (string, error) { return formatSSE("start-step", p) }

// FinishStep signals the end of a step.
type FinishStep struct{}

func (p FinishStep) Format() (string, error) { return formatSSE("finish-step", p) }

// MessageMetadata carries additional metadata for the message.
type MessageMetadata struct {
	Metadata map[string]any `json:"messageMetadata"`
}

func (p MessageMetadata) Format() (string, error) { return formatSSE("message-metadata", p) }

// ---------------------------------------------------------------------------
// Text streaming (start → delta* → end)
// ---------------------------------------------------------------------------

// TextStart signals the beginning of a text content block.
type TextStart struct {
	ID               string         `json:"id"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p TextStart) Format() (string, error) { return formatSSE("text-start", p) }

// TextDelta streams incremental text content.
type TextDelta struct {
	ID               string         `json:"id"`
	Delta            string         `json:"delta"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p TextDelta) Format() (string, error) { return formatSSE("text-delta", p) }

// TextEnd signals the end of a text content block.
type TextEnd struct {
	ID               string         `json:"id"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p TextEnd) Format() (string, error) { return formatSSE("text-end", p) }

// ---------------------------------------------------------------------------
// Reasoning streaming (start → delta* → end)
// ---------------------------------------------------------------------------

// ReasoningStart signals the beginning of a reasoning/thinking block.
type ReasoningStart struct {
	ID               string         `json:"id"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p ReasoningStart) Format() (string, error) { return formatSSE("reasoning-start", p) }

// ReasoningDelta streams incremental reasoning content.
type ReasoningDelta struct {
	ID               string         `json:"id"`
	Delta            string         `json:"delta"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p ReasoningDelta) Format() (string, error) { return formatSSE("reasoning-delta", p) }

// ReasoningEnd signals the end of a reasoning block.
type ReasoningEnd struct {
	ID               string         `json:"id"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

func (p ReasoningEnd) Format() (string, error) { return formatSSE("reasoning-end", p) }

// ---------------------------------------------------------------------------
// Tool Input (streaming tool call arguments)
// ---------------------------------------------------------------------------

// ToolInputStart signals the beginning of a tool call.
type ToolInputStart struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
}

func (p ToolInputStart) Format() (string, error) { return formatSSE("tool-input-start", p) }

// ToolInputDelta streams incremental tool call argument text.
type ToolInputDelta struct {
	ToolCallID     string `json:"toolCallId"`
	InputTextDelta string `json:"inputTextDelta"`
}

func (p ToolInputDelta) Format() (string, error) { return formatSSE("tool-input-delta", p) }

// ToolInputAvailable signals a complete tool call with parsed arguments.
type ToolInputAvailable struct {
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Input      map[string]any `json:"input"`
}

func (p ToolInputAvailable) Format() (string, error) { return formatSSE("tool-input-available", p) }

// ---------------------------------------------------------------------------
// Tool Output
// ---------------------------------------------------------------------------

// ToolOutputAvailable carries the result of a successful tool execution.
type ToolOutputAvailable struct {
	ToolCallID string `json:"toolCallId"`
	Output     any    `json:"output"`
}

func (p ToolOutputAvailable) Format() (string, error) { return formatSSE("tool-output-available", p) }

// ToolOutputError carries a tool execution error.
type ToolOutputError struct {
	ToolCallID string `json:"toolCallId"`
	ErrorText  string `json:"errorText"`
}

func (p ToolOutputError) Format() (string, error) { return formatSSE("tool-output-error", p) }

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

// Error signals an error in the stream.
type Error struct {
	ErrorText string `json:"errorText"`
}

func (p Error) Format() (string, error) { return formatSSE("error", p) }

// ---------------------------------------------------------------------------
// Custom Data (type: "data-{suffix}")
// ---------------------------------------------------------------------------

// DataPart carries custom framework data. The wire type is "data-{Suffix}".
type DataPart struct {
	Suffix    string `json:"-"`
	ID        string `json:"id,omitempty"`
	Data      any    `json:"data"`
	Transient bool   `json:"transient,omitempty"`
}

func (p DataPart) Format() (string, error) {
	return formatSSE("data-"+p.Suffix, p)
}

// ---------------------------------------------------------------------------
// Framework extension constructors (transmitted via DataPart)
// ---------------------------------------------------------------------------

// NewInterruptPart creates a DataPart signaling an agent interrupt.
func NewInterruptPart(interruptData any) DataPart {
	return DataPart{Suffix: "interrupt", Data: interruptData}
}

// NewTransferPart creates a DataPart signaling an agent transfer.
func NewTransferPart(agentName string) DataPart {
	return DataPart{Suffix: "transfer", Data: map[string]any{"agent": agentName}}
}

// ---------------------------------------------------------------------------
// Wire format helper
// ---------------------------------------------------------------------------

// formatSSE serializes a typed chunk to SSE wire format: "data: {json}\n\n"
// It injects the "type" field at the beginning of the JSON object.
func formatSSE(typeName string, v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("stream: marshal %s: %w", typeName, err)
	}
	// Empty struct → {"type":"..."}
	if string(raw) == "{}" {
		return fmt.Sprintf("data: {\"type\":\"%s\"}\n\n", typeName), nil
	}
	// Insert "type" at beginning: {"type":"...","existingField":...}
	return fmt.Sprintf("data: {\"type\":\"%s\",%s\n\n", typeName, raw[1:]), nil
}
