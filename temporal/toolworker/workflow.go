package toolworker

import (
	"time"

	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/temporal"
	"github.com/nickqiaoo/tadk/tool"
)

const (
	WorkflowName = "TemporalToolWorkflow"
	SignalResume = "resume"
	SignalCancel = "cancel"
	QueryState   = "state"

	defaultActivityStartToCloseTimeout = 10 * time.Minute
)

// WorkflowInput identifies one durable tool execution.
type WorkflowInput struct {
	ToolName       string
	AppName        string
	SessionID      string
	UserID         string
	AgentName      string
	Branch         string
	Address        string
	FunctionCallID string
	Args           map[string]any

	// ActivityStartToCloseTimeout bounds how long a single tool segment may run.
	// Zero uses defaultActivityStartToCloseTimeout.
	ActivityStartToCloseTimeout time.Duration
	// ActivityRetryPolicy controls how Temporal retries a failed segment.
	// Nil leaves the Temporal server default in place. Callers invoking
	// non-idempotent tools should set MaximumAttempts=1 to avoid double
	// side-effects.
	ActivityRetryPolicy *sdktemporal.RetryPolicy
}

type workflowState struct {
	Status            temporal.Status
	Result            map[string]any
	Control           *tool.Control
	PendingInterrupts map[string]*session.PendingInterrupt
	Error             string
}

// RunWorkflow executes one Temporal-backed durable tool until completion or interrupt.
func RunWorkflow(ctx workflow.Context, input *WorkflowInput) (*temporal.ToolResult, error) {
	state := &workflowState{
		Status: temporal.StatusRunning,
	}
	if err := workflow.SetQueryHandler(ctx, QueryState, func() (*temporal.ToolState, error) {
		return &temporal.ToolState{
			Status:            state.Status,
			Result:            cloneMap(state.Result),
			Control:           cloneControl(state.Control),
			PendingInterrupts: clonePendingInterrupts(state.PendingInterrupts),
			Error:             state.Error,
		}, nil
	}); err != nil {
		return nil, err
	}

	resumeChan := workflow.GetSignalChannel(ctx, SignalResume)
	cancelChan := workflow.GetSignalChannel(ctx, SignalCancel)

	startToClose := input.ActivityStartToCloseTimeout
	if startToClose <= 0 {
		startToClose = defaultActivityStartToCloseTimeout
	}
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: startToClose,
		RetryPolicy:         input.ActivityRetryPolicy,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var resumeData map[string]any
	for {
		if checkCancel(ctx, cancelChan) {
			state.Status = temporal.StatusCancelled
			return &temporal.ToolResult{Result: state.Result, Control: state.Control, Cancelled: true}, nil
		}

		var output RunSegmentOutput
		err := workflow.ExecuteActivity(ctx, "RunSegment", &RunSegmentInput{
			ToolName:          input.ToolName,
			AppName:           input.AppName,
			SessionID:         input.SessionID,
			UserID:            input.UserID,
			AgentName:         input.AgentName,
			Branch:            input.Branch,
			Address:           input.Address,
			FunctionCallID:    input.FunctionCallID,
			Args:              cloneMap(input.Args),
			ResumeData:        cloneMap(resumeData),
			PendingInterrupts: clonePendingInterrupts(state.PendingInterrupts),
		}).Get(ctx, &output)
		if err != nil {
			state.Status = temporal.StatusFailed
			state.Error = err.Error()
			return &temporal.ToolResult{Result: state.Result, Control: state.Control, Error: state.Error}, nil
		}

		state.Result = cloneMap(output.Result)
		state.Control = cloneControl(output.Control)
		if output.Error != "" {
			state.Status = temporal.StatusFailed
			state.Error = output.Error
			return &temporal.ToolResult{Result: state.Result, Control: state.Control, Error: state.Error}, nil
		}
		if len(output.PendingInterrupts) == 0 {
			state.Status = temporal.StatusCompleted
			state.PendingInterrupts = nil
			return &temporal.ToolResult{Result: state.Result, Control: state.Control}, nil
		}

		state.Status = temporal.StatusInterrupted
		state.Result = nil
		state.PendingInterrupts = clonePendingInterrupts(output.PendingInterrupts)

		var cancelled bool
		resumeData, cancelled = waitResumeOrCancel(ctx, resumeChan, cancelChan)
		if cancelled {
			state.Status = temporal.StatusCancelled
			return &temporal.ToolResult{Result: state.Result, Control: state.Control, Cancelled: true}, nil
		}

		state.Status = temporal.StatusRunning
		state.Error = ""
	}
}

func checkCancel(ctx workflow.Context, cancelChan workflow.ReceiveChannel) bool {
	var cancelled bool
	selector := workflow.NewSelector(ctx)
	selector.AddReceive(cancelChan, func(c workflow.ReceiveChannel, more bool) {
		cancelled = true
	})
	selector.AddDefault(func() {})
	selector.Select(ctx)
	return cancelled
}

func waitResumeOrCancel(ctx workflow.Context, resumeChan, cancelChan workflow.ReceiveChannel) (map[string]any, bool) {
	var resumeData map[string]any
	var cancelled bool

	selector := workflow.NewSelector(ctx)
	selector.AddReceive(resumeChan, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, &resumeData)
	})
	selector.AddReceive(cancelChan, func(c workflow.ReceiveChannel, more bool) {
		cancelled = true
	})
	selector.Select(ctx)

	return resumeData, cancelled
}

func clonePendingInterrupts(src map[string]*session.PendingInterrupt) map[string]*session.PendingInterrupt {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]*session.PendingInterrupt, len(src))
	for id, cur := range src {
		if cur == nil {
			continue
		}
		cloned := *cur
		cloned.ToolArgs = cloneMap(cur.ToolArgs)
		dst[id] = &cloned
	}
	return dst
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneControl(src *tool.Control) *tool.Control {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}
