package agent

import (
	"context"

	"github.com/google/uuid"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

/*
InvocationContext represents the context of an agent invocation.

An invocation:
 1. Starts with a user message and ends with a final response.
 2. Can contain one or multiple agent calls.
 3. Is handled by runner.Run().

An agent call:
 1. Is handled by agent.Run().
 2. Ends when agent.Run() ends.

An agent call can contain one or multiple steps.
For example, LLM agent runs steps in a loop until:
 1. A final response is generated.
 2. The agent transfers to another agent.
 3. The run control signals an end to the invocation.

A step:
 1. Calls the LLM only once and yields its response.
 2. Calls the tools and yields their responses if requested.

The summarization of the function response is considered another step, since
it is another LLM call.
A step ends when it's done calling LLM and tools, or if the run control signals
an end to the invocation at any time.

	┌─────────────────────── invocation ──────────────────────────┐
	┌──────────── llm_agent_call_1 ────────────┐ ┌─ agent_call_2 ─┐
	┌──── step_1 ────────┐ ┌───── step_2 ──────┐
	[call_llm] [call_tool] [call_llm] [transfer]
*/
type InvocationContext interface {
	// Context returns the underlying Go context.
	Context() context.Context

	// AgentName returns the name of the agent for this invocation.
	AgentName() string

	// Session of the current invocation context.
	Session() session.Session

	InvocationID() string

	// Branch of the invocation context.
	// The format is like agent_1.agent_2.agent_3, where agent_1 is the parent
	// of agent_2, and agent_2 is the parent of agent_3.
	//
	// Branch is used when multiple sub-agents shouldn't see their peer agents'
	// conversation history.
	//
	// Applicable to parallel agent because its sub-agents run concurrently.
	Branch() string
	// Address is the identifier used by the resume system to match a pending
	// interrupt (`Checkpoint.PendingInterrupts[*].AgentAddress`) to the agent
	// that should consume its resume data. It is compared for exact string
	// equality; agents that do not own an interrupt are skipped.
	//
	// Semantics differ by caller:
	//   - Top-level agent invocations (runner entry, agent transfer targets):
	//     Address == AgentName(), i.e. the hierarchy is flat.
	//   - Workflow children (loopagent / parallelagent / sequentialagent
	//     sub-agents): Address is the dotted path "parent.child[.grandchild]",
	//     built by workflowinternal.SubAgentAddress so each iteration position
	//     has a distinct address and resume data cannot cross sub-agents.
	//
	// When extending interrupt handling, use Address() (not AgentName()) for
	// checkpoint lookups; Address is the stable key and AgentName alone is
	// ambiguous under workflow nesting.
	Address() string

	// UserContent that started this invocation.
	UserContent() *message.Message

	// RunConfig returns the stable invocation configuration shared by all
	// contexts derived from the same invocation. The returned pointer is
	// guaranteed non-nil and MUST be treated as read-only; use DeriveRunConfig
	// if a mutated copy is required.
	RunConfig() *RunConfig

	// RunState returns the per-invocation runtime state snapshot. The returned
	// pointer is guaranteed non-nil and MUST be treated as read-only; use
	// DeriveRunState if a mutated copy is required.
	RunState() *RunState

	// Tree returns the parentmap tree for agent navigation.
	Tree() ParentTree

	// WithContext returns a new instance with the underlying Go context replaced.
	WithContext(ctx context.Context) InvocationContext

	// WithRunConfig returns a copy with the invocation configuration replaced.
	// The caller transfers ownership of cfg to the new context; cfg MUST NOT
	// be mutated afterwards.
	WithRunConfig(cfg *RunConfig) InvocationContext

	// WithRunState returns a copy with the runtime state replaced. The caller
	// transfers ownership of state to the new context; state MUST NOT be
	// mutated afterwards.
	WithRunState(state *RunState) InvocationContext
}

// ParentTree defines the read-only view of an agent tree needed by agents.
type ParentTree interface {
	ParentOf(name string) Agent
	ChildrenOf(name string) []Agent
	SiblingsOf(name string) []Agent
	IsDescendant(childName, ancestorName string) bool
	RootAgent(cur Agent) Agent
}

// InvocationContextParams is the concrete input for constructing an
// InvocationContext implementation. RunConfig and RunState are optional; a
// nil value is treated as a fresh empty configuration/state. The caller
// transfers ownership of the pointers to the context, which subsequently
// shares them read-only across derived contexts.
type InvocationContextParams struct {
	Session session.Session

	Branch    string
	Address   string
	AgentName string

	UserContent  *message.Message
	RunConfig    *RunConfig
	RunState     *RunState
	InvocationID string
	Tree         ParentTree
}

// NewInvocationContext constructs the framework's canonical invocation context.
// Nil RunConfig/RunState in params are normalized to empty non-nil values; the
// caller-provided pointers are adopted without cloning, so callers must not
// mutate them after this call.
func NewInvocationContext(ctx context.Context, params InvocationContextParams) InvocationContext {
	if params.InvocationID == "" {
		params.InvocationID = "e-" + uuid.NewString()
	}
	if params.RunConfig == nil {
		params.RunConfig = &RunConfig{}
	}
	if params.RunState == nil {
		params.RunState = &RunState{}
	}

	return &invocationContextImpl{
		ctx:    ctx,
		params: params,
	}
}

type invocationContextImpl struct {
	ctx    context.Context
	params InvocationContextParams
}

func (c *invocationContextImpl) AgentName() string {
	return c.params.AgentName
}

func (c *invocationContextImpl) Session() session.Session {
	return c.params.Session
}

func (c *invocationContextImpl) InvocationID() string {
	return c.params.InvocationID
}

func (c *invocationContextImpl) Branch() string {
	return c.params.Branch
}

func (c *invocationContextImpl) Address() string {
	return c.params.Address
}

func (c *invocationContextImpl) UserContent() *message.Message {
	return c.params.UserContent
}

func (c *invocationContextImpl) RunConfig() *RunConfig {
	return c.params.RunConfig
}

func (c *invocationContextImpl) RunState() *RunState {
	return c.params.RunState
}

func (c *invocationContextImpl) Tree() ParentTree {
	return c.params.Tree
}

func (c *invocationContextImpl) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *invocationContextImpl) WithContext(ctx context.Context) InvocationContext {
	newCtx := *c
	newCtx.ctx = ctx
	return &newCtx
}

func (c *invocationContextImpl) WithRunConfig(cfg *RunConfig) InvocationContext {
	newCtx := *c
	if cfg == nil {
		cfg = &RunConfig{}
	}
	newCtx.params.RunConfig = cfg
	return &newCtx
}

func (c *invocationContextImpl) WithRunState(state *RunState) InvocationContext {
	newCtx := *c
	if state == nil {
		state = &RunState{}
	}
	newCtx.params.RunState = state
	return &newCtx
}
