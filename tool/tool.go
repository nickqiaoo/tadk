// Package tool defines the interfaces for tools that can be called by an agent.
// A tool is a piece of code that performs a specific task. You can either define
// your own custom tools or use built-in ones, for example, GoogleSearch.
package tool

import (
	"context"

	"github.com/nickqiaoo/tadk/agent"
)

// Tool defines the interface for a callable tool.
type Tool interface {
	// Name returns the name of the tool.
	Name() string
	// Description returns a description of the tool.
	Description() string
	// Schema returns the JSON Schema describing the tool's input parameters.
	Schema() map[string]any
	// Execute runs the tool with the given arguments and returns the result
	// together with optional flow-control signals.
	Execute(ctx Context, args map[string]any) (any, *Control, error)
}

// Context defines the interface for the context passed to a tool when it's
// called. It provides access to invocation-specific information and allows
// the tool to interact with the agent's state.
type Context interface {
	// Context returns the underlying Go context.
	Context() context.Context

	// InvocationContext returns the current agent invocation context.
	InvocationContext() agent.InvocationContext

	// FunctionCallID returns the unique identifier of the function call
	// that triggered this tool execution.
	FunctionCallID() string

	// Interrupt pauses tool execution and waits for external input.
	Interrupt(hint string, payload any) error

	// ResumeData returns the resume state for the current tool call.
	// Returns (isResumeTarget, hasData, data):
	//   - isResumeTarget: true if this tool call is a resume target
	//   - hasData: true if resume data is available
	//   - data: the resume data provided by the user
	ResumeData() (isResumeTarget bool, hasData bool, data any)

	// InterruptState returns the state that was saved during the interrupt.
	// This is different from ResumeData which contains user-provided data.
	// Returns (hasState, state):
	//   - hasState: true if interrupt state is available
	//   - state: the state saved during the interrupt
	InterruptState() (hasState bool, state any)
}

// Control carries flow-control signals produced by a tool execution.
type Control struct {
	Terminal        bool
	TransferToAgent string
	ExitToParent    bool
}



// Toolset is an interface for a collection of tools. It allows grouping
// related tools together and providing them to an agent.
type Toolset interface {
	// Name returns the name of the toolset.
	Name() string
	// Tools returns a list of tools in the toolset. It is invoked exactly
	// once per invocation, at prefix-build time; any I/O (e.g. MCP server
	// discovery) happens here. The returned list is frozen into the
	// invocation's Prefix and reused for every step, so it must be stable
	// for the lifetime of a single invocation. Implementations may use
	// InvocationContext for init-time reads but must not depend on per-step
	// state, or LLM-provider prompt caches will never hit.
	Tools(ctx agent.InvocationContext) ([]Tool, error)
}

// Predicate is a function which decides whether a tool should be exposed to LLM.
type Predicate func(ctx agent.InvocationContext, tool Tool) bool

// StringPredicate is a helper that creates a Predicate from a string slice.
func StringPredicate(allowedTools []string) Predicate {
	m := make(map[string]bool)
	for _, t := range allowedTools {
		m[t] = true
	}

	return func(ctx agent.InvocationContext, tool Tool) bool {
		return m[tool.Name()]
	}
}

// FilterToolset returns a Toolset that filters the tools in the given Toolset
// using the given predicate.
func FilterToolset(toolset Toolset, predicate Predicate) Toolset {
	if toolset == nil {
		panic("toolset must not be nil")
	}
	if predicate == nil {
		panic("predicate must not be nil")
	}

	return &filteredToolset{
		toolset:   toolset,
		predicate: predicate,
	}
}

type filteredToolset struct {
	toolset   Toolset
	predicate Predicate
}

func (f *filteredToolset) Name() string {
	return f.toolset.Name()
}

func (f *filteredToolset) Tools(ctx agent.InvocationContext) ([]Tool, error) {
	tools, err := f.toolset.Tools(ctx)
	if err != nil {
		return nil, err
	}
	var filtered []Tool
	for _, tool := range tools {
		if f.predicate(ctx, tool) {
			filtered = append(filtered, tool)
		}
	}
	return filtered, nil
}
