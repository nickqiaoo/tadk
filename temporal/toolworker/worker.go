package toolworker

import (
	"fmt"

	"go.temporal.io/sdk/client"
	tworker "go.temporal.io/sdk/worker"
	tworkflow "go.temporal.io/sdk/workflow"

	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

// Config describes how to construct a Temporal worker for durable tools.
//
// Client is optional. If provided, the caller owns its lifecycle and Close()
// will not close it. Otherwise ClientOptions is used to dial a client that the
// worker owns.
type Config struct {
	Client         client.Client
	ClientOptions  client.Options
	TaskQueue      string
	SessionService session.Service
}

// Worker hosts the Temporal workflow and activities for registered tools.
type Worker struct {
	client     client.Client
	ownsClient bool
	worker     tworker.Worker
	taskQueue  string
	sessionSvc session.Service
	registry   *ToolRegistry
}

// New constructs a Temporal worker.
func New(cfg Config) (*Worker, error) {
	if cfg.SessionService == nil {
		return nil, fmt.Errorf("session service is required")
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
	w := tworker.New(c, cfg.TaskQueue, tworker.Options{})

	return &Worker{
		client:     c,
		ownsClient: ownsClient,
		worker:     w,
		taskQueue:  cfg.TaskQueue,
		sessionSvc: cfg.SessionService,
		registry:   NewToolRegistry(),
	}, nil
}

// Client exposes the underlying Temporal client so callers can share it with
// other components (e.g. a durable-tool runner) instead of dialing a second one.
func (w *Worker) Client() client.Client {
	return w.client
}

// TaskQueue returns the task queue the worker polls.
func (w *Worker) TaskQueue() string {
	return w.taskQueue
}

// RegisterTool registers a durable tool definition with this worker.
func (w *Worker) RegisterTool(t tool.Tool) {
	w.registry.Register(t)
}

// Start registers workflow/activity handlers and starts polling.
func (w *Worker) Start() error {
	w.registerHandlers()
	return w.worker.Start()
}

// Run registers workflow/activity handlers and blocks on the Temporal interrupt channel.
func (w *Worker) Run() error {
	w.registerHandlers()
	return w.worker.Run(tworker.InterruptCh())
}

// Stop stops the underlying worker.
func (w *Worker) Stop() {
	w.worker.Stop()
}

// Close releases worker resources. The client is only closed if the worker
// dialed it itself; a caller-provided client is left untouched.
func (w *Worker) Close() {
	if w.worker != nil {
		w.worker.Stop()
	}
	if w.ownsClient && w.client != nil {
		w.client.Close()
	}
}

func (w *Worker) registerHandlers() {
	w.worker.RegisterWorkflowWithOptions(RunWorkflow, tworkflow.RegisterOptions{Name: WorkflowName})
	w.worker.RegisterActivity(&Activities{
		registry:   w.registry,
		sessionSvc: w.sessionSvc,
	})
}

// ToolRegistry stores durable tools by name.
type ToolRegistry struct {
	tools map[string]tool.Tool
}

// NewToolRegistry constructs an empty registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]tool.Tool)}
}

// Register stores or replaces a tool definition by name.
func (r *ToolRegistry) Register(t tool.Tool) {
	if t == nil {
		return
	}
	r.tools[t.Name()] = t
}

// Get looks up a tool definition by name.
func (r *ToolRegistry) Get(name string) (tool.Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}
