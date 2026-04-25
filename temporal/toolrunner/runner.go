package toolrunner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"

	"github.com/nickqiaoo/tadk/temporal"
	"github.com/nickqiaoo/tadk/temporal/toolworker"
)

// Config describes a Temporal workflow client bound to one durable tool definition.
//
// Client is optional. If provided, the caller owns its lifecycle and Close()
// is a no-op on the client. Otherwise ClientOptions is used to dial a new one.
type Config struct {
	ToolName      string
	Client        client.Client
	ClientOptions client.Options
	TaskQueue     string
}

// RunRequest is the input required to start one durable tool workflow.
type RunRequest struct {
	AppName        string
	SessionID      string
	UserID         string
	AgentName      string
	Branch         string
	Address        string
	FunctionCallID string
	Args           map[string]any

	// ActivityStartToCloseTimeout bounds how long one tool segment may run
	// inside the workflow. Zero leaves the workflow's default in place.
	ActivityStartToCloseTimeout time.Duration
	// ActivityRetryPolicy controls retry behavior for each tool segment.
	// Nil leaves the Temporal server default in place.
	ActivityRetryPolicy *sdktemporal.RetryPolicy
}

// Runner starts, resumes, queries, and waits for Temporal-backed durable tool workflows.
type Runner struct {
	toolName   string
	client     client.Client
	ownsClient bool
	taskQueue  string
}

// New constructs a workflow runner.
func New(cfg Config) (*Runner, error) {
	if cfg.ToolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	if cfg.TaskQueue == "" {
		return nil, fmt.Errorf("task queue is required")
	}

	c := cfg.Client
	ownsClient := false
	if c == nil {
		if cfg.ClientOptions.HostPort == "" {
			return nil, fmt.Errorf("client or client host port is required")
		}
		dialed, err := client.Dial(cfg.ClientOptions)
		if err != nil {
			return nil, fmt.Errorf("dial temporal client: %w", err)
		}
		c = dialed
		ownsClient = true
	}
	return &Runner{
		toolName:   cfg.ToolName,
		client:     c,
		ownsClient: ownsClient,
		taskQueue:  cfg.TaskQueue,
	}, nil
}

// Run starts a new Temporal-backed durable tool workflow and returns the workflow ID.
func (r *Runner) Run(ctx context.Context, req RunRequest) (string, error) {
	workflowID := fmt.Sprintf("%s-%s-%s", r.toolName, req.SessionID, shortID())
	_, err := r.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: r.taskQueue,
	}, toolworker.WorkflowName, &toolworker.WorkflowInput{
		ToolName:                    r.toolName,
		AppName:                     req.AppName,
		SessionID:                   req.SessionID,
		UserID:                      req.UserID,
		AgentName:                   req.AgentName,
		Branch:                      req.Branch,
		Address:                     req.Address,
		FunctionCallID:              req.FunctionCallID,
		Args:                        cloneMap(req.Args),
		ActivityStartToCloseTimeout: req.ActivityStartToCloseTimeout,
		ActivityRetryPolicy:         req.ActivityRetryPolicy,
	})
	if err != nil {
		return "", fmt.Errorf("start workflow: %w", err)
	}
	return workflowID, nil
}

// Resume signals a suspended workflow with resume data.
func (r *Runner) Resume(ctx context.Context, workflowID string, resumeData map[string]any) error {
	return r.client.SignalWorkflow(ctx, workflowID, "", toolworker.SignalResume, cloneMap(resumeData))
}

// Cancel signals a workflow to stop.
func (r *Runner) Cancel(ctx context.Context, workflowID string) error {
	return r.client.SignalWorkflow(ctx, workflowID, "", toolworker.SignalCancel, nil)
}

// GetState queries the workflow state.
func (r *Runner) GetState(ctx context.Context, workflowID string) (*temporal.ToolState, error) {
	resp, err := r.client.QueryWorkflow(ctx, workflowID, "", toolworker.QueryState)
	if err != nil {
		return nil, fmt.Errorf("query workflow: %w", err)
	}
	var state temporal.ToolState
	if err := resp.Get(&state); err != nil {
		return nil, fmt.Errorf("decode workflow state: %w", err)
	}
	return &state, nil
}

// Wait blocks until the workflow completes or fails.
func (r *Runner) Wait(ctx context.Context, workflowID string) (*temporal.ToolResult, error) {
	we := r.client.GetWorkflow(ctx, workflowID, "")
	var result temporal.ToolResult
	if err := we.Get(ctx, &result); err != nil {
		return nil, fmt.Errorf("wait workflow: %w", err)
	}
	return &result, nil
}

// Close releases the Temporal client if the runner owns it.
func (r *Runner) Close() {
	if r.ownsClient && r.client != nil {
		r.client.Close()
	}
}

func shortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b)
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
