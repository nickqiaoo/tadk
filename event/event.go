// Package event defines the unified runtime event types used across the framework.
// It merges the former model.AssistantMessageEvent stream with event.Event
// into a single, type-safe interface hierarchy.
package event

import (
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/resume"
)

// Type identifies the kind of runtime event.
type Type string

const (
	TypeAgentStart Type = "agent_start"
	TypeAgentEnd   Type = "agent_end"

	TypeTurnStart Type = "turn_start"

	TypeMessageStart Type = "message_start"
	TypeMessageEnd   Type = "message_end"

	TypeTextDelta     Type = "text_delta"
	TypeThinkingDelta Type = "thinking_delta"
	TypeToolCallDelta Type = "toolcall_delta"

	TypeToolExecutionStart Type = "tool_execution_start"
	TypeToolExecutionEnd   Type = "tool_execution_end"

	TypeInterrupt Type = "interrupt"

	TypeAgentTransferRequest Type = "agent_transfer_request"
)

// Event is the unified runtime event interface.
type Event interface {
	EventType() Type
	Meta() Meta
	Control() Control
	// WithMeta returns a new Event with the given Meta replaced and the original
	// Control preserved.
	WithMeta(Meta) Event
}

// Meta carries common runtime metadata for every event.
type Meta struct {
	Author       string
	Branch       string
	InvocationID string
}

// Control carries runtime control signals for every event.
type Control struct {
	TransferToAgent string
	ExitToParent    bool
	Terminal        bool
}

// Frame provides the common Meta and Control fields for all event types.
type Frame struct {
	m Meta
	c Control
}

// Meta returns the event metadata.
func (f Frame) Meta() Meta { return f.m }

// Control returns the event control signals.
func (f Frame) Control() Control { return f.c }

// Option mutates an event's Meta or Control during construction.
type Option func(*Meta, *Control)

func WithAuthor(author string) Option {
	return func(m *Meta, c *Control) { m.Author = author }
}

func WithBranch(branch string) Option {
	return func(m *Meta, c *Control) { m.Branch = branch }
}

func WithInvocationID(id string) Option {
	return func(m *Meta, c *Control) { m.InvocationID = id }
}

func WithTransferToAgent(name string) Option {
	return func(m *Meta, c *Control) { c.TransferToAgent = name }
}

func WithExitToParent(v bool) Option {
	return func(m *Meta, c *Control) { c.ExitToParent = v }
}

func WithTerminal(v bool) Option {
	return func(m *Meta, c *Control) { c.Terminal = v }
}

// WithMetaControl returns an Option that overwrites both Meta and Control.
func WithMetaControl(meta Meta, ctrl Control) Option {
	return func(m *Meta, c *Control) {
		*m = meta
		*c = ctrl
	}
}

// WithControl returns an Option that overwrites the entire Control struct.
// Prefer this over individual WithTransferToAgent / WithExitToParent /
// WithTerminal options when you already have a Control value in hand.
func WithControl(ctrl Control) Option {
	return func(_ *Meta, c *Control) { *c = ctrl }
}

func applyOptions(m *Meta, c *Control, opts []Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(m, c)
		}
	}
}

// WithMeta returns a new Event with the given Meta replaced and the original
// Control preserved. It delegates to the event's own WithMeta method.
func WithMeta(ev Event, meta Meta) Event {
	if ev == nil {
		return nil
	}
	return ev.WithMeta(meta)
}

// ---------------------------------------------------------------------------
// Concrete event types
// ---------------------------------------------------------------------------

type AgentStart struct{ Frame }

func (e *AgentStart) EventType() Type { return TypeAgentStart }

func (e *AgentStart) WithMeta(meta Meta) Event { return &AgentStart{Frame: Frame{m: meta, c: e.Control()}} }

// AgentEnd is the terminal event for a Runner.Run invocation. It carries the
// pointer slice of messages emitted during the run (a projection of the
// MessageEnd stream, not re-persisted) and the terminal error, if any.
type AgentEnd struct {
	Frame
	Messages []*message.Message
	Err      error
}

func (e *AgentEnd) EventType() Type { return TypeAgentEnd }

func (e *AgentEnd) WithMeta(meta Meta) Event {
	return &AgentEnd{Frame: Frame{m: meta, c: e.Control()}, Messages: e.Messages, Err: e.Err}
}

type TurnStart struct {
	Frame
	TurnIndex int
}

func (e *TurnStart) EventType() Type { return TypeTurnStart }

func (e *TurnStart) WithMeta(meta Meta) Event {
	return &TurnStart{Frame: Frame{m: meta, c: e.Control()}, TurnIndex: e.TurnIndex}
}

type MessageStart struct {
	Frame
	Message *message.Message
}

func (e *MessageStart) EventType() Type { return TypeMessageStart }

func (e *MessageStart) WithMeta(meta Meta) Event {
	return &MessageStart{Frame: Frame{m: meta, c: e.Control()}, Message: e.Message}
}

type MessageEnd struct {
	Frame
	Message *message.Message
}

func (e *MessageEnd) EventType() Type { return TypeMessageEnd }

func (e *MessageEnd) WithMeta(meta Meta) Event {
	return &MessageEnd{Frame: Frame{m: meta, c: e.Control()}, Message: e.Message}
}

type TextDelta struct {
	Frame
	ContentIndex int
	Delta        string
	Partial      *message.Message
}

func (e *TextDelta) EventType() Type { return TypeTextDelta }

func (e *TextDelta) WithMeta(meta Meta) Event {
	return &TextDelta{
		Frame:        Frame{m: meta, c: e.Control()},
		ContentIndex: e.ContentIndex,
		Delta:        e.Delta,
		Partial:      e.Partial,
	}
}

type ThinkingDelta struct {
	Frame
	ContentIndex int
	Delta        string
	Signature    string
	Redacted     bool
	Partial      *message.Message
}

func (e *ThinkingDelta) EventType() Type { return TypeThinkingDelta }

func (e *ThinkingDelta) WithMeta(meta Meta) Event {
	return &ThinkingDelta{
		Frame:        Frame{m: meta, c: e.Control()},
		ContentIndex: e.ContentIndex,
		Delta:        e.Delta,
		Signature:    e.Signature,
		Redacted:     e.Redacted,
		Partial:      e.Partial,
	}
}

type ToolCallDelta struct {
	Frame
	ContentIndex     int
	ToolCallID       string
	ToolName         string
	Delta            string
	ThoughtSignature string
	Partial          *message.Message
}

func (e *ToolCallDelta) EventType() Type { return TypeToolCallDelta }

func (e *ToolCallDelta) WithMeta(meta Meta) Event {
	return &ToolCallDelta{
		Frame:            Frame{m: meta, c: e.Control()},
		ContentIndex:     e.ContentIndex,
		ToolCallID:       e.ToolCallID,
		ToolName:         e.ToolName,
		Delta:            e.Delta,
		ThoughtSignature: e.ThoughtSignature,
		Partial:          e.Partial,
	}
}

type ToolExecutionStart struct {
	Frame
	ToolCallID string
	ToolName   string
	Args       any
}

func (e *ToolExecutionStart) EventType() Type { return TypeToolExecutionStart }

func (e *ToolExecutionStart) WithMeta(meta Meta) Event {
	return &ToolExecutionStart{
		Frame:      Frame{m: meta, c: e.Control()},
		ToolCallID: e.ToolCallID,
		ToolName:   e.ToolName,
		Args:       e.Args,
	}
}

type ToolExecutionEnd struct {
	Frame
	ToolCallID string
	ToolName   string
	Result     any
	IsError    bool
}

func (e *ToolExecutionEnd) EventType() Type { return TypeToolExecutionEnd }

func (e *ToolExecutionEnd) WithMeta(meta Meta) Event {
	return &ToolExecutionEnd{
		Frame:      Frame{m: meta, c: e.Control()},
		ToolCallID: e.ToolCallID,
		ToolName:   e.ToolName,
		Result:     e.Result,
		IsError:    e.IsError,
	}
}

type Interrupt struct {
	Frame
	Interrupt *resume.InterruptData
}

func (e *Interrupt) EventType() Type { return TypeInterrupt }

func (e *Interrupt) WithMeta(meta Meta) Event {
	return &Interrupt{Frame: Frame{m: meta, c: e.Control()}, Interrupt: e.Interrupt}
}

type AgentTransferRequest struct {
	Frame
	TargetAgent string
}

func (e *AgentTransferRequest) EventType() Type { return TypeAgentTransferRequest }

func (e *AgentTransferRequest) WithMeta(meta Meta) Event {
	return &AgentTransferRequest{Frame: Frame{m: meta, c: e.Control()}, TargetAgent: e.TargetAgent}
}

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

func NewAgentStart(opts ...Option) Event {
	e := &AgentStart{}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewAgentEnd(messages []*message.Message, err error, opts ...Option) Event {
	e := &AgentEnd{Messages: messages, Err: err}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewTurnStart(turnIndex int, opts ...Option) Event {
	e := &TurnStart{TurnIndex: turnIndex}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewMessageStart(msg *message.Message, opts ...Option) Event {
	e := &MessageStart{Message: msg}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewMessageEnd(msg *message.Message, opts ...Option) Event {
	e := &MessageEnd{Message: msg}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewTextDelta(contentIndex int, delta string, partial *message.Message, opts ...Option) Event {
	e := &TextDelta{ContentIndex: contentIndex, Delta: delta, Partial: partial}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewThinkingDelta(contentIndex int, delta, signature string, redacted bool, partial *message.Message, opts ...Option) Event {
	e := &ThinkingDelta{ContentIndex: contentIndex, Delta: delta, Signature: signature, Redacted: redacted, Partial: partial}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewToolCallDelta(contentIndex int, toolCallID, toolName, delta string, partial *message.Message, opts ...Option) Event {
	e := &ToolCallDelta{ContentIndex: contentIndex, ToolCallID: toolCallID, ToolName: toolName, Delta: delta, Partial: partial}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewToolExecutionStart(toolCallID, toolName string, args any, opts ...Option) Event {
	e := &ToolExecutionStart{ToolCallID: toolCallID, ToolName: toolName, Args: args}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewToolExecutionEnd(toolCallID, toolName string, result any, isError bool, opts ...Option) Event {
	e := &ToolExecutionEnd{ToolCallID: toolCallID, ToolName: toolName, Result: result, IsError: isError}
	applyOptions(&e.m, &e.c, opts)
	return e
}

func NewInterrupt(interrupt *resume.InterruptData, opts ...Option) Event {
	e := &Interrupt{Interrupt: interrupt}
	applyOptions(&e.m, &e.c, opts)
	return e
}

// NewAgentTransferRequest builds a control event that signals an agent
// handoff. The target is mirrored onto Control.TransferToAgent so that the
// runner — and any other consumer — can detect transfer intent uniformly via
// Event.Control(), the same field used by tool-triggered transfers.
func NewAgentTransferRequest(targetAgent string, opts ...Option) Event {
	e := &AgentTransferRequest{TargetAgent: targetAgent}
	applyOptions(&e.m, &e.c, opts)
	if targetAgent != "" {
		e.c.TransferToAgent = targetAgent
	}
	return e
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// IsFinalResponse reports whether the event represents a final assistant response.
func IsFinalResponse(ev Event) bool {
	msgEnd, ok := ev.(*MessageEnd)
	if !ok || msgEnd == nil || msgEnd.Message == nil {
		return false
	}
	msg := msgEnd.Message
	if msg.Role != message.RoleAssistant || msg.HasToolCalls() {
		return false
	}
	return true
}

