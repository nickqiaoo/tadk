package temporaltool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"

	"github.com/nickqiaoo/tadk/internal/utils"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/temporal"
	temprunner "github.com/nickqiaoo/tadk/temporal/toolrunner"
	"github.com/nickqiaoo/tadk/temporal/toolworker"
	"github.com/nickqiaoo/tadk/tool"
)

const defaultPollInterval = 100 * time.Millisecond

// Config describes how a regular tool is wrapped as a durable Temporal tool.
//
// At least one of Client, ClientOptions (with HostPort) must be provided so
// the runner can reach Temporal. If Worker is set, the wrapped tool is
// automatically registered on it and the worker's client is reused when
// Client is not explicitly provided, avoiding a second dial.
type Config struct {
	Wrapped       tool.Tool
	Worker        *toolworker.Worker
	Client        client.Client
	ClientOptions client.Options
	TaskQueue     string
	PollInterval  time.Duration

	// ActivityStartToCloseTimeout bounds how long one tool segment may run
	// inside the Temporal workflow. Zero uses the workflow default.
	ActivityStartToCloseTimeout time.Duration
	// ActivityRetryPolicy controls retry behavior for each tool segment. Nil
	// leaves the Temporal server default in place. Set MaximumAttempts=1
	// for non-idempotent tools to avoid duplicate side-effects.
	ActivityRetryPolicy *sdktemporal.RetryPolicy
}

// New wraps an existing tool as a Temporal-backed durable tool.
func New(cfg Config) (tool.Tool, error) {
	if cfg.Wrapped == nil {
		return nil, fmt.Errorf("wrapped tool is required")
	}

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	runnerCfg := temprunner.Config{
		ToolName:      cfg.Wrapped.Name(),
		Client:        cfg.Client,
		ClientOptions: cfg.ClientOptions,
		TaskQueue:     cfg.TaskQueue,
	}
	if runnerCfg.Client == nil && cfg.Worker != nil {
		runnerCfg.Client = cfg.Worker.Client()
	}
	if runnerCfg.TaskQueue == "" && cfg.Worker != nil {
		runnerCfg.TaskQueue = cfg.Worker.TaskQueue()
	}

	r, err := temprunner.New(runnerCfg)
	if err != nil {
		return nil, err
	}

	if cfg.Worker != nil {
		cfg.Worker.RegisterTool(cfg.Wrapped)
	}

	return &durableTool{
		wrapped:                     cfg.Wrapped,
		runner:                      r,
		pollInterval:                pollInterval,
		activityStartToCloseTimeout: cfg.ActivityStartToCloseTimeout,
		activityRetryPolicy:         cfg.ActivityRetryPolicy,
	}, nil
}

type durableTool struct {
	wrapped                     tool.Tool
	runner                      *temprunner.Runner
	pollInterval                time.Duration
	activityStartToCloseTimeout time.Duration
	activityRetryPolicy         *sdktemporal.RetryPolicy
}

func (t *durableTool) Name() string {
	return t.wrapped.Name()
}

func (t *durableTool) Description() string {
	return t.wrapped.Description()
}

func (t *durableTool) Schema() map[string]any {
	return t.wrapped.Schema()
}

func (t *durableTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	workflowID, err := t.startOrSignalResume(ctx, args)
	if err != nil {
		return nil, nil, err
	}

	for {
		select {
		case <-ctx.Context().Done():
			return nil, nil, ctx.Context().Err()
		default:
		}

		state, err := t.runner.GetState(ctx.Context(), workflowID)
		if err != nil {
			return nil, nil, err
		}

		switch state.Status {
		case temporal.StatusRunning:
			if err := sleepWithContext(ctx.Context(), t.pollInterval); err != nil {
				return nil, nil, err
			}
		case temporal.StatusCompleted:
			return cloneMap(state.Result), cloneControl(state.Control), nil
		case temporal.StatusCancelled:
			return cloneMap(state.Result), cloneControl(state.Control), nil
		case temporal.StatusInterrupted:
			return nil, cloneControl(state.Control), t.buildInterrupt(ctx, workflowID, state.PendingInterrupts)
		case temporal.StatusFailed:
			if state.Error != "" {
				return nil, cloneControl(state.Control), fmt.Errorf("workflow failed: %s", state.Error)
			}
			return nil, cloneControl(state.Control), fmt.Errorf("workflow failed")
		default:
			if err := sleepWithContext(ctx.Context(), t.pollInterval); err != nil {
				return nil, nil, err
			}
		}
	}
}

func (t *durableTool) startOrSignalResume(ctx tool.Context, args map[string]any) (string, error) {
	isResumeTarget, _, resumeData := ctx.ResumeData()
	if isResumeTarget {
		stored, err := loadStoredInterruptState(ctx)
		if err != nil {
			return "", err
		}
		payload, err := buildResumePayload(resumeData, stored.InterruptIDs)
		if err != nil {
			return "", err
		}
		if err := t.runner.Resume(ctx.Context(), stored.WorkflowID, payload); err != nil {
			return "", fmt.Errorf("signal resume workflow: %w", err)
		}
		return stored.WorkflowID, nil
	}

	workflowID, err := t.runner.Run(ctx.Context(), temprunner.RunRequest{
		AppName:                     ctx.InvocationContext().Session().AppName(),
		SessionID:                   ctx.InvocationContext().Session().ID(),
		UserID:                      ctx.InvocationContext().Session().UserID(),
		AgentName:                   ctx.InvocationContext().AgentName(),
		Branch:                      ctx.InvocationContext().Branch(),
		Address:                     ctx.InvocationContext().Address(),
		FunctionCallID:              ctx.FunctionCallID(),
		Args:                        cloneMap(args),
		ActivityStartToCloseTimeout: t.activityStartToCloseTimeout,
		ActivityRetryPolicy:         t.activityRetryPolicy,
	})
	if err != nil {
		return "", fmt.Errorf("start temporal workflow: %w", err)
	}
	return workflowID, nil
}

func (t *durableTool) buildInterrupt(toolCtx tool.Context, workflowID string, pending map[string]*session.PendingInterrupt) error {
	info := map[string]any{
		"type":        "temporal_workflow",
		"tool":        t.Name(),
		"workflow_id": workflowID,
		"interrupts":  pendingInterruptSummaries(pending),
	}
	state := map[string]any{
		"workflow_id":   workflowID,
		"interrupt_ids": pendingInterruptIDs(pending),
	}
	address := utils.ToolAddress(toolCtx.InvocationContext().Address(), toolCtx.FunctionCallID())
	return resume.StatefulInterrupt(toolCtx.Context(), address, info, state)
}

type storedInterruptState struct {
	WorkflowID   string   `json:"workflow_id"`
	InterruptIDs []string `json:"interrupt_ids"`
}

func loadStoredInterruptState(ctx tool.Context) (*storedInterruptState, error) {
	hasState, state := ctx.InterruptState()
	if !hasState {
		return nil, fmt.Errorf("missing temporal workflow state for resume")
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal temporal workflow state: %w", err)
	}
	var stored storedInterruptState
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, fmt.Errorf("decode temporal workflow state: %w", err)
	}
	if stored.WorkflowID == "" {
		return nil, fmt.Errorf("missing workflow id in temporal workflow state")
	}
	return &stored, nil
}

func buildResumePayload(data any, interruptIDs []string) (map[string]any, error) {
	if len(interruptIDs) == 0 {
		return nil, fmt.Errorf("missing interrupt ids for temporal workflow resume")
	}

	if m, ok := data.(map[string]any); ok {
		if containsAnyKey(m, interruptIDs) {
			return cloneMap(m), nil
		}
		if len(interruptIDs) == 1 {
			return map[string]any{interruptIDs[0]: cloneMap(m)}, nil
		}
		return nil, fmt.Errorf(
			"resume data for multiple temporal interrupts must be keyed by interrupt id; expected keys %v, got %v",
			sortedCopy(interruptIDs), mapKeys(m),
		)
	}

	if len(interruptIDs) != 1 {
		return nil, fmt.Errorf(
			"resume data for multiple temporal interrupts must be keyed by interrupt id; expected keys %v, got %T",
			sortedCopy(interruptIDs), data,
		)
	}
	return map[string]any{interruptIDs[0]: data}, nil
}

func containsAnyKey(m map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func pendingInterruptIDs(pending map[string]*session.PendingInterrupt) []string {
	if len(pending) == 0 {
		return nil
	}
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	return ids
}

func pendingInterruptSummaries(pending map[string]*session.PendingInterrupt) []map[string]any {
	if len(pending) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(pending))
	for _, cur := range pending {
		if cur == nil {
			continue
		}
		out = append(out, map[string]any{
			"id":      cur.ID,
			"address": cur.Address,
			"info":    cur.Info,
		})
	}
	return out
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}
