package agent

import (
	"fmt"
	"iter"

	"go.opentelemetry.io/otel/trace"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/telemetry"
	"github.com/nickqiaoo/tadk/message"
)

// Agent is the base interface which all agents must implement.
//
// Agents are created with ADK constructors to ensure correct
// init & configuration.
// The constructors are available in this package and its subpackages.
// For example: llmagent.New, workflow agents, remote agent or
// agent.New.
type Agent interface {
	Name() string
	Description() string
	Run(InvocationContext) iter.Seq2[event.Event, error]
	SubAgents() []Agent
	AgentType() Type
}

// TransferPolicyCarrier is implemented by agents that expose a transfer policy.
type TransferPolicyCarrier interface {
	TransferToParentDisabled() bool
}

// BaseAgent provides the canonical Name/Description/SubAgents accessors for
// Agent implementations. Concrete agent types embed BaseAgent to get the
// standard accessors; use NewBaseAgent to construct one with duplicate-
// subagent validation applied.
type BaseAgent struct {
	name        string
	description string
	subAgents   []Agent
}

// NewBaseAgent validates subAgents and returns a BaseAgent. Subagents must be
// unique by pointer identity.
func NewBaseAgent(name, description string, subAgents []Agent) (BaseAgent, error) {
	seen := make(map[Agent]bool)
	for _, sa := range subAgents {
		if seen[sa] {
			return BaseAgent{}, fmt.Errorf("error creating agent: subagent %q appears multiple times in subAgents", sa.Name())
		}
		seen[sa] = true
	}
	return BaseAgent{name: name, description: description, subAgents: subAgents}, nil
}

// Name returns the agent's configured name.
func (b BaseAgent) Name() string { return b.name }

// Description returns the agent's human-readable description.
func (b BaseAgent) Description() string { return b.description }

// SubAgents returns the agent's declared sub-agents.
func (b BaseAgent) SubAgents() []Agent { return b.subAgents }

// WrapRun instruments an agent's raw run function with the framework's
// invoke_agent telemetry span and centralized event metadata injection.
// Every Agent implementation should delegate its Run method to this helper so
// that Author/Branch/InvocationID defaults are applied uniformly and events
// never leak without metadata. The wrapped run receives a context with
// AgentName set to a.Name().
func WrapRun(ctx InvocationContext, a Agent, run func(InvocationContext) iter.Seq2[event.Event, error]) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		spanCtx, span := telemetry.StartInvokeAgentSpan(ctx.Context(), a, ctx.Session().ID(), ctx.InvocationID())
		yield, endSpan := telemetry.WrapYield(span, yield, func(span trace.Span, output event.Event, err error) {
			telemetry.TraceAgentResult(span, telemetry.TraceAgentResultParams{Error: err})
		})
		defer endSpan()
		ctx := DeriveInvocationContext(ctx, InvocationContextOverrides{
			Context:   spanCtx,
			AgentName: a.Name(),
		})
		for output, err := range run(ctx) {
			if output != nil {
				output = withDefaultMeta(output, ctx)
			}
			if !yield(output, err) {
				return
			}
		}
	}
}

// New creates an Agent with custom logic defined by the Run function in cfg.
func New(cfg Config) (Agent, error) {
	base, err := NewBaseAgent(cfg.Name, cfg.Description, cfg.SubAgents)
	if err != nil {
		return nil, err
	}
	return &agent{
		BaseAgent: base,
		run:       cfg.Run,
	}, nil
}

// Config is the configuration for creating a new Agent.
type Config struct {
	// Name must be a non-empty string, unique within the agent tree.
	// Agent name cannot be "user", since it's reserved for end-user's input.
	Name string
	// Description of the agent's capability.
	//
	// LLM uses this to determine whether to delegate control to the agent.
	// One-line description is enough and preferred.
	Description string
	// SubAgents are the child agents that this agent can delegate tasks to.
	// ADK will automatically set a parent of each sub-agent to this agent to
	// allow agent transferring across the tree.
	SubAgents []Agent
	// Run is the function that defines the agent's behavior.
	Run func(InvocationContext) iter.Seq2[event.Event, error]
}

type agent struct {
	BaseAgent
	run func(InvocationContext) iter.Seq2[event.Event, error]
}

func (a *agent) AgentType() Type {
	return TypeCustomAgent
}

func (a *agent) Run(ctx InvocationContext) iter.Seq2[event.Event, error] {
	if a.run == nil {
		return func(yield func(event.Event, error) bool) {}
	}
	return WrapRun(ctx, a, a.run)
}

func withDefaultMeta(ev event.Event, ctx InvocationContext) event.Event {
	m := ev.Meta()
	if isUserRole(ev) {
		m.Author = string(message.RoleUser)
	} else if m.Author == "" {
		m.Author = ctx.AgentName()
	}
	if m.Branch == "" {
		m.Branch = ctx.Branch()
	}
	if m.InvocationID == "" {
		m.InvocationID = ctx.InvocationID()
	}
	return event.WithMeta(ev, m)
}

func isUserRole(ev event.Event) bool {
	switch e := ev.(type) {
	case *event.MessageStart:
		return e.Message != nil && e.Message.Role == message.RoleUser
	case *event.MessageEnd:
		return e.Message != nil && e.Message.Role == message.RoleUser
	}
	return false
}
