package agent

import (
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// Extension is the unified hook system that replaces old runner plugins,
// agent lifecycle callbacks, and LLM request/tool hook customization.
//
// Framework core logic (building conversation history, adding tools, injecting
// system prompts) is handled internally. Extensions are for user customization
// on top of that.
//
// All methods are optional — implement only the hooks you need by embedding
// DefaultExtension.
type Extension interface {
	RunnerExtension
	TurnExtension
	ModelExtension
	ToolExtension
}

// RunControl allows runner-level extensions to influence invocation flow.
type RunControl struct {
	// EndInvocation stops any planned agent calls for this invocation.
	EndInvocation bool
}

// RunnerExtension hooks into the runner lifecycle.
type RunnerExtension interface {
	// Name returns the extension's identifier.
	Name() string

	// BeforeRun is called before the agent invocation starts.
	// Return a non-nil Message to short-circuit the run with that content.
	BeforeRun(ctx InvocationContext, ctrl *RunControl) (*message.Message, error)

	// AfterRun is called after the agent invocation completes.
	AfterRun(ctx InvocationContext, ctrl *RunControl) error

	// OnEvent is called for each runtime agent event produced during the run.
	// The extension can transform or filter the event.
	OnEvent(ctx InvocationContext, event event.Event) (event.Event, error)
}

// TurnExtension hooks into the agent turn loop.
type TurnExtension interface {
	Name() string

	// BeforeTurn is called at the start of each agent loop turn, before
	// the LLM call. Extensions can modify ctrl to inject messages into
	// the conversation or influence turn flow.
	BeforeTurn(ctx InvocationContext, ctrl *TurnControl) error

	// AfterTurn is called after each turn completes (after tool execution).
	// Extensions can modify ctrl to inject messages into the conversation
	// or force the agent to continue even if it would otherwise stop.
	//
	// Messages added to ctrl.Messages are emitted as events and persisted
	// to the session, preserving prompt cache prefix consistency.
	AfterTurn(ctx InvocationContext, ctrl *TurnControl) error
}

// ModelExtension hooks into LLM calls.
type ModelExtension interface {
	Name() string

	// BeforeModelCall is called after the framework builds the LLM request
	// but before sending it. Extensions can modify the request (e.g., inject
	// additional context, transform messages, add headers).
	//
	// If it returns a non-nil Message or error, the actual model call is
	// skipped and the returned message/error is used instead.
	BeforeModelCall(ctx InvocationContext, req *model.Request) (*message.Message, error)

	// AfterModelCall is called after the model returns either a final assistant
	// message or an error. Extensions can inspect or transform the message in
	// place, replace it, or replace the model error.
	//
	// Returning (nil, nil) keeps the current message/error.
	AfterModelCall(ctx InvocationContext, msg *message.Message, llmResponseError error) (*message.Message, error)
}

// ToolExtension hooks into tool execution.
type ToolExtension interface {
	Name() string

	// BeforeToolCall is called before a tool is executed.
	//
	// Extensions may mutate the tool call in place. If the method returns a
	// non-nil ToolResult or error, the actual tool call is skipped and the
	// returned result/error is used instead.
	BeforeToolCall(ctx InvocationContext, call *message.ToolCall) (*message.ToolResult, error)

	// AfterToolCall is called after a tool finishes executing, whether it
	// returned a result or an error. Extensions may mutate the result in place
	// or replace the result/error by returning non-nil values.
	AfterToolCall(ctx InvocationContext, call *message.ToolCall, result *message.ToolResult, toolError error) (*message.ToolResult, error)
}

// TurnControl allows extensions to inject messages and control turn flow.
// It is passed to BeforeTurn and AfterTurn hooks. Multiple extensions share
// the same TurnControl instance and can all append to Messages.
type TurnControl struct {
	// Messages to inject into the conversation. These are emitted as
	// MessageStart/MessageEnd events and persisted to the session by the runner.
	Messages []*message.Message
	// Continue forces another turn even if the agent would otherwise stop.
	Continue bool
	// EndInvocation stops any planned agent calls for this invocation.
	EndInvocation bool
}

// DefaultExtension provides no-op implementations of all Extension methods.
// Embed this in your extension to only override the hooks you need.
type DefaultExtension struct{}

func (DefaultExtension) Name() string { return "" }
func (DefaultExtension) BeforeRun(ctx InvocationContext, ctrl *RunControl) (*message.Message, error) {
	return nil, nil
}
func (DefaultExtension) AfterRun(ctx InvocationContext, ctrl *RunControl) error { return nil }
func (DefaultExtension) OnEvent(ctx InvocationContext, event event.Event) (event.Event, error) {
	return event, nil
}
func (DefaultExtension) BeforeTurn(ctx InvocationContext, ctrl *TurnControl) error { return nil }
func (DefaultExtension) AfterTurn(ctx InvocationContext, ctrl *TurnControl) error  { return nil }
func (DefaultExtension) BeforeModelCall(ctx InvocationContext, req *model.Request) (*message.Message, error) {
	return nil, nil
}
func (DefaultExtension) AfterModelCall(ctx InvocationContext, msg *message.Message, llmResponseError error) (*message.Message, error) {
	return nil, nil
}
func (DefaultExtension) BeforeToolCall(ctx InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
	return nil, nil
}
func (DefaultExtension) AfterToolCall(ctx InvocationContext, call *message.ToolCall, result *message.ToolResult, toolError error) (*message.ToolResult, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Extension helpers
// ---------------------------------------------------------------------------

// ForEach iterates over extensions and applies fn to each one.
// If fn returns (true, nil), iteration stops early (short-circuit).
// If fn returns a non-nil error, iteration stops and the error is returned.
func ForEach[E any](exts []E, fn func(E) (stop bool, err error)) error {
	for _, ext := range exts {
		var zero E
		if any(ext) == any(zero) {
			continue
		}
		stop, err := fn(ext)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return nil
}

// AsRunnerExtensions filters a slice of Extensions into RunnerExtensions.
func AsRunnerExtensions(exts []Extension) []RunnerExtension {
	return filterExtensions(exts, func(e Extension) (RunnerExtension, bool) {
		v, ok := e.(RunnerExtension)
		return v, ok && v != nil
	})
}

// AsTurnExtensions filters a slice of Extensions into TurnExtensions.
func AsTurnExtensions(exts []Extension) []TurnExtension {
	return filterExtensions(exts, func(e Extension) (TurnExtension, bool) {
		v, ok := e.(TurnExtension)
		return v, ok && v != nil
	})
}

// AsModelExtensions filters a slice of Extensions into ModelExtensions.
func AsModelExtensions(exts []Extension) []ModelExtension {
	return filterExtensions(exts, func(e Extension) (ModelExtension, bool) {
		v, ok := e.(ModelExtension)
		return v, ok && v != nil
	})
}

// AsToolExtensions filters a slice of Extensions into ToolExtensions.
func AsToolExtensions(exts []Extension) []ToolExtension {
	return filterExtensions(exts, func(e Extension) (ToolExtension, bool) {
		v, ok := e.(ToolExtension)
		return v, ok && v != nil
	})
}

func filterExtensions[E any](exts []Extension, pick func(Extension) (E, bool)) []E {
	var out []E
	for _, ext := range exts {
		if ext == nil {
			continue
		}
		if v, ok := pick(ext); ok {
			out = append(out, v)
		}
	}
	return out
}

// NewExtensionContext is a no-op that returns the provided InvocationContext.
// It exists for backward compatibility; callers may use the context directly.
func NewExtensionContext(ctx InvocationContext) InvocationContext {
	return ctx
}

// RunConfigFromExtensionContext returns the stable invocation configuration.
// The returned pointer is non-nil and read-only.
func RunConfigFromExtensionContext(ctx InvocationContext) *RunConfig {
	if ctx == nil {
		return &RunConfig{}
	}
	return ctx.RunConfig()
}

// RunStateFromExtensionContext returns the invocation runtime state snapshot.
// The returned pointer is non-nil and read-only.
func RunStateFromExtensionContext(ctx InvocationContext) *RunState {
	if ctx == nil {
		return &RunState{}
	}
	return ctx.RunState()
}
