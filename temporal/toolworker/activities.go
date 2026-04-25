package toolworker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/internal/utils"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/session"
	adktool "github.com/nickqiaoo/tadk/tool"
)

// RunSegmentInput describes one durable tool segment execution.
type RunSegmentInput struct {
	ToolName          string
	AppName           string
	SessionID         string
	UserID            string
	AgentName         string
	Branch            string
	Address           string
	FunctionCallID    string
	Args              map[string]any
	ResumeData        map[string]any
	PendingInterrupts map[string]*session.PendingInterrupt
}

// RunSegmentOutput is the durable result of a single tool segment execution.
type RunSegmentOutput struct {
	Result            map[string]any                       `json:"result,omitempty"`
	Control           *adktool.Control                     `json:"control,omitempty"`
	PendingInterrupts map[string]*session.PendingInterrupt `json:"pendingInterrupts,omitempty"`
	Error             string                               `json:"error,omitempty"`
}

// Activities contains the activity implementations used by the Temporal worker.
type Activities struct {
	registry   *ToolRegistry
	sessionSvc session.Service
}

// RunSegment executes one durable tool segment until completion or interrupt.
func (a *Activities) RunSegment(ctx context.Context, input *RunSegmentInput) (*RunSegmentOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	curTool, ok := a.registry.Get(input.ToolName)
	if !ok {
		return nil, fmt.Errorf("temporal tool %q not registered", input.ToolName)
	}

	resp, err := a.sessionSvc.Get(ctx, &session.GetRequest{
		AppName:   input.AppName,
		UserID:    input.UserID,
		SessionID: input.SessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	runState := &agent.RunState{
		AgentStateService: a.sessionSvc,
	}
	invCtx := agent.NewInvocationContext(ctx, agent.InvocationContextParams{
		Session:   resp.Session,
		AgentName: input.AgentName,
		Branch:    input.Branch,
		Address:   input.Address,
		RunState:  runState,
	})

	opts, err := toolContextOptions(input.ResumeData, input.FunctionCallID, input.PendingInterrupts)
	if err != nil {
		return nil, err
	}
	toolCtx := toolinternal.NewToolContext(invCtx, input.FunctionCallID, opts...)

	output, ctrl, execErr := curTool.Execute(toolCtx, cloneMap(input.Args))
	if resume.IsInterrupt(execErr) {
		var sig *resume.InterruptSignal
		if !errors.As(execErr, &sig) || sig == nil {
			return nil, fmt.Errorf("interrupt error missing signal")
		}
		pending, err := buildPendingInterrupts(sig.ToInterruptData(), input.FunctionCallID, curTool.Name(), input.Args, input.Address)
		if err != nil {
			return nil, err
		}
		return &RunSegmentOutput{
			Control:           cloneControl(ctrl),
			PendingInterrupts: pending,
		}, nil
	}
	if execErr != nil {
		return &RunSegmentOutput{
			Control: cloneControl(ctrl),
			Error:   execErr.Error(),
		}, nil
	}

	return &RunSegmentOutput{
		Result:  normalizeResult(output),
		Control: cloneControl(ctrl),
	}, nil
}

func toolContextOptions(resumeData map[string]any, functionCallID string, pending map[string]*session.PendingInterrupt) ([]toolinternal.ToolContextOption, error) {
	if len(pending) == 0 {
		return nil, nil
	}

	data, err := resumePayloadForPendingInterrupts(resumeData, functionCallID, pending)
	if err != nil {
		return nil, err
	}
	opts := []toolinternal.ToolContextOption{
		toolinternal.WithResumeData(true, data),
	}
	if state := interruptStateForPendingInterrupts(pending); state != nil {
		opts = append(opts, toolinternal.WithInterruptState(state))
	}
	return opts, nil
}

func resumePayloadForPendingInterrupts(resumeData map[string]any, functionCallID string, pending map[string]*session.PendingInterrupt) (any, error) {
	if len(pending) == 0 {
		return nil, nil
	}
	if resumeData == nil {
		return nil, fmt.Errorf(
			"missing resume data for temporal tool call %q: expected one of function_call_id or interrupt ids %v",
			functionCallID, sortedInterruptIDs(pending),
		)
	}

	if len(pending) == 1 {
		for interruptID := range pending {
			if data, ok := utils.ResumeDataForToolCall(resumeData, functionCallID, interruptID); ok {
				return data, nil
			}
		}
	}

	if data, ok := resumeData[functionCallID]; ok {
		return data, nil
	}

	matched := make(map[string]any)
	for interruptID := range pending {
		if data, ok := resumeData[interruptID]; ok {
			matched[interruptID] = data
		}
	}
	if len(matched) > 0 {
		return matched, nil
	}

	return nil, fmt.Errorf(
		"no matching resume data for temporal tool call %q: expected key %q or one of interrupt ids %v, got %v",
		functionCallID, functionCallID, sortedInterruptIDs(pending), sortedResumeKeys(resumeData),
	)
}

func sortedInterruptIDs(pending map[string]*session.PendingInterrupt) []string {
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedResumeKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func interruptStateForPendingInterrupts(pending map[string]*session.PendingInterrupt) any {
	if len(pending) == 0 {
		return nil
	}
	if len(pending) == 1 {
		for _, cur := range pending {
			if cur != nil {
				return cur.State
			}
		}
		return nil
	}

	state := make(map[string]any, len(pending))
	for id, cur := range pending {
		if cur == nil {
			continue
		}
		state[id] = cur.State
	}
	if len(state) == 0 {
		return nil
	}
	return state
}

func buildPendingInterrupts(interrupt *resume.InterruptData, functionCallID, toolName string, args map[string]any, defaultAgentAddress string) (map[string]*session.PendingInterrupt, error) {
	if interrupt == nil || len(interrupt.Contexts) == 0 {
		return nil, nil
	}

	pending := make(map[string]*session.PendingInterrupt, len(interrupt.Contexts))
	for _, interruptCtx := range interrupt.Contexts {
		if interruptCtx == nil {
			continue
		}

		callID := interruptToolCallID(interruptCtx)
		agentAddress, addressCallID := splitToolAddress(interruptCtx.Address)
		if callID == "" {
			callID = addressCallID
		}
		if callID == "" {
			callID = functionCallID
		}
		if callID == "" {
			return nil, fmt.Errorf("interrupt %q missing function_call_id", interruptCtx.ID)
		}
		if agentAddress == "" {
			agentAddress = defaultAgentAddress
		}

		address := interruptCtx.Address
		if _, toolID := splitToolAddress(address); toolID == "" {
			address = utils.ToolAddress(agentAddress, callID)
		}

		pending[interruptCtx.ID] = &session.PendingInterrupt{
			ID:           interruptCtx.ID,
			Address:      address,
			AgentAddress: agentAddress,
			ToolCallID:   callID,
			ToolName:     toolName,
			ToolArgs:     cloneMap(args),
			State:        interrupt.ID2State[interruptCtx.ID],
			Info:         interruptCtx.Info,
			CreatedAt:    time.Now().Format(time.RFC3339Nano),
		}
	}
	return pending, nil
}

func interruptToolCallID(ctx *resume.InterruptContext) string {
	for cur := ctx; cur != nil; cur = cur.Parent {
		info, ok := cur.Info.(map[string]any)
		if !ok {
			continue
		}
		callID, _ := info["function_call_id"].(string)
		if callID != "" {
			return callID
		}
	}
	return ""
}

func normalizeResult(output any) map[string]any {
	if output == nil {
		return nil
	}
	if result, ok := output.(map[string]any); ok {
		return cloneMap(result)
	}
	return map[string]any{"result": output}
}

func splitToolAddress(address string) (agentAddress, toolCallID string) {
	const marker = ":tool:"
	if idx := strings.LastIndex(address, marker); idx >= 0 {
		return address[:idx], address[idx+len(marker):]
	}
	const prefix = "tool:"
	if strings.HasPrefix(address, prefix) {
		return "", strings.TrimPrefix(address, prefix)
	}
	return address, ""
}
