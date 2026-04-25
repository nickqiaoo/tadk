package toolworker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/temporal"
	"github.com/nickqiaoo/tadk/tool"
)

type approvalTool struct{}

func (approvalTool) Name() string {
	return "approval"
}

func (approvalTool) Description() string {
	return "Requires approval before continuing"
}

func (approvalTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request": map[string]any{"type": "string"},
		},
	}
}

func (approvalTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	isResumeTarget, hasData, data := ctx.ResumeData()
	if !isResumeTarget {
		address := fmt.Sprintf("%s:tool:%s", ctx.InvocationContext().Address(), ctx.FunctionCallID())
		return nil, nil, resume.StatefulInterrupt(ctx.Context(), address, map[string]any{
			"type": "approval",
		}, map[string]any{
			"request": args["request"],
		})
	}
	if !hasData {
		return map[string]any{"approved": false}, nil, nil
	}

	hasState, state := ctx.InterruptState()
	return map[string]any{
		"approved":    data,
		"saved_state": state,
		"had_state":   hasState,
	}, &tool.Control{Terminal: true}, nil
}

func TestRunWorkflowInterruptResume(t *testing.T) {
	t.Parallel()

	sessionSvc := session.InMemoryService()
	const (
		appName   = "temporal-tool-test-app"
		userID    = "test-user"
		sessionID = "session-1"
	)

	if _, err := sessionSvc.Create(context.Background(), &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := NewToolRegistry()
	registry.Register(approvalTool{})

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(RunWorkflow)
	env.RegisterActivity(&Activities{
		registry:   registry,
		sessionSvc: sessionSvc,
	})

	env.RegisterDelayedCallback(func() {
		value, err := env.QueryWorkflow(QueryState)
		if err != nil {
			t.Fatalf("QueryWorkflow() error = %v", err)
		}
		var state temporal.ToolState
		if err := value.Get(&state); err != nil {
			t.Fatalf("query decode error = %v", err)
		}
		if state.Status != temporal.StatusInterrupted {
			t.Fatalf("state.Status = %q, want %q", state.Status, temporal.StatusInterrupted)
		}
		if len(state.PendingInterrupts) != 1 {
			t.Fatalf("len(state.PendingInterrupts) = %d, want 1", len(state.PendingInterrupts))
		}

		resumeData := make(map[string]any)
		for interruptID := range state.PendingInterrupts {
			resumeData[interruptID] = map[string]any{"approved": true}
		}
		env.SignalWorkflow(SignalResume, resumeData)
	}, time.Second)

	env.ExecuteWorkflow(RunWorkflow, &WorkflowInput{
		ToolName:       "approval",
		AppName:        appName,
		SessionID:      sessionID,
		UserID:         userID,
		AgentName:      "root",
		Address:        "root",
		FunctionCallID: "call-approval",
		Args: map[string]any{
			"request": "ship it",
		},
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error = %v", err)
	}

	var result temporal.ToolResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult() error = %v", err)
	}
	if result.Control == nil || !result.Control.Terminal {
		t.Fatalf("result.Control = %#v, want Terminal=true", result.Control)
	}

	savedState, ok := result.Result["saved_state"].(map[string]any)
	if !ok {
		t.Fatalf("saved_state = %#v, want map[string]any", result.Result["saved_state"])
	}
	if got := savedState["request"]; got != "ship it" {
		t.Fatalf("saved_state.request = %v, want ship it", got)
	}
	approved, ok := result.Result["approved"].(map[string]any)
	if !ok {
		t.Fatalf("approved = %#v, want map[string]any", result.Result["approved"])
	}
	if got := approved["approved"]; got != true {
		t.Fatalf("approved.approved = %v, want true", got)
	}
}
