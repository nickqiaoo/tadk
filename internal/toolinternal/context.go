package toolinternal

import (
	"context"

	"github.com/google/uuid"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/tool"
)

// ToolContextOption configures a tool context.
type ToolContextOption func(*toolContext)

// WithResumeData sets the resume data on the tool context.
func WithResumeData(isResumeTarget bool, data any) ToolContextOption {
	return func(c *toolContext) {
		c.resumeTarget = isResumeTarget
		c.resumeData = data
	}
}

// WithInterruptState sets the interrupt state on the tool context.
func WithInterruptState(state any) ToolContextOption {
	return func(c *toolContext) {
		c.interruptState = state
	}
}

// NewToolContext constructs the concrete implementation of tool.Context.
func NewToolContext(ctx agent.InvocationContext, functionCallID string, opts ...ToolContextOption) tool.Context {
	if functionCallID == "" {
		functionCallID = uuid.NewString()
	}

	tc := &toolContext{
		invCtx:         ctx,
		functionCallID: functionCallID,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(tc)
		}
	}
	return tc
}

type toolContext struct {
	invCtx         agent.InvocationContext
	functionCallID string
	resumeTarget   bool
	resumeData     any
	interruptState any
}

func (c *toolContext) Context() context.Context {
	return c.invCtx.Context()
}

func (c *toolContext) InvocationContext() agent.InvocationContext {
	return c.invCtx
}

func (c *toolContext) FunctionCallID() string {
	return c.functionCallID
}

func (c *toolContext) Interrupt(hint string, payload any) error {
	return resume.Interrupt(c.invCtx.Context(), c.invCtx.Address(), map[string]any{
		"function_call_id": c.functionCallID,
		"hint":             hint,
		"payload":          payload,
	})
}

func (c *toolContext) ResumeData() (bool, bool, any) {
	return c.resumeTarget, c.resumeData != nil, c.resumeData
}

func (c *toolContext) InterruptState() (bool, any) {
	return c.interruptState != nil, c.interruptState
}
