// Package interrupt provides decorators for tools that require user approval
// or other forms of interruption before execution.
package interrupt

import (
	"fmt"

	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/tool"
)

// ApprovalInfo contains information about the approval request
// that will be shown to the user.
type ApprovalInfo struct {
	Type    string         `json:"type"`
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Message string         `json:"message"`
}

// approvalTool wraps a tool to require user approval before execution.
type approvalTool struct {
	wrapped tool.Tool
	message string
}

// WithApproval wraps a tool to require user approval before execution.
// When the tool is called, it will return an interrupt signal with the
// provided message. The tool will only execute when resumed with
// {"approved": true} in the resume data.
func WithApproval(t tool.Tool, message string) tool.Tool {
	return &approvalTool{wrapped: t, message: message}
}

// ===== tool.Tool interface =====

func (a *approvalTool) Name() string {
	return a.wrapped.Name()
}

func (a *approvalTool) Description() string {
	return a.wrapped.Description()
}

func (a *approvalTool) Schema() map[string]any {
	return a.wrapped.Schema()
}

func (a *approvalTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	// Check if this is a resume call
	isResumeTarget, hasData, data := ctx.ResumeData()
	if isResumeTarget && hasData {
		// Validate approval
		dataMap, ok := data.(map[string]any)
		if !ok {
			return map[string]any{
				"status":  "error",
				"message": "invalid resume data format",
			}, nil, nil
		}

		approved, _ := dataMap["approved"].(bool)
		if !approved {
			return map[string]any{
				"status":  "rejected",
				"message": "user rejected the operation",
			}, nil, nil
		}

		// User approved, execute the wrapped tool
		output, ctrl, err := a.wrapped.Execute(ctx, args)
		if err != nil {
			return nil, ctrl, err
		}
		if result, ok := output.(map[string]any); ok {
			return result, ctrl, nil
		}
		return map[string]any{"result": output}, ctrl, nil
	}

	// First call: return interrupt signal
	info := ApprovalInfo{
		Type:    "approval",
		Tool:    a.wrapped.Name(),
		Args:    args,
		Message: a.message,
	}
	address := fmt.Sprintf("%s:tool:%s", ctx.InvocationContext().Address(), ctx.FunctionCallID())
	return nil, nil, resume.Interrupt(ctx.Context(), address, info)
}

func (a *approvalTool) AugmentRequest(ctx tool.Context, req *model.Request) error {
	augmenter, ok := a.wrapped.(toolinternal.RequestAugmenter)
	if !ok {
		return nil
	}
	return augmenter.AugmentRequest(ctx, req)
}

var _ toolinternal.RequestAugmenter = (*approvalTool)(nil)
