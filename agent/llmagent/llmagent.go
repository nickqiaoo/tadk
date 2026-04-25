package llmagent

import (
	"fmt"
	"iter"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/llminternal"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/tool"
)

// New is a constructor for LLMAgent.
func New(cfg Config) (agent.Agent, error) {
	base, err := agent.NewBaseAgent(cfg.Name, cfg.Description, cfg.SubAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}
	return &llmAgent{
		BaseAgent: base,
		cfg: llminternal.Config{
			Model:                    cfg.Model,
			ModelID:                  cfg.ModelID,
			Config:                   cfg.Config,
			Tools:                    cfg.Tools,
			Toolsets:                 cfg.Toolsets,
			DisallowTransferToParent: cfg.DisallowTransferToParent,
			DisallowTransferToPeers:  cfg.DisallowTransferToPeers,
			InputSchema:              cfg.InputSchema,
			OutputSchema:             cfg.OutputSchema,
			// TODO: internal type for includeContents
			IncludeContents:           string(cfg.IncludeContents),
			Instruction:               cfg.Instruction,
			InstructionProvider:       llminternal.InstructionProvider(cfg.InstructionProvider),
			GlobalInstruction:         cfg.GlobalInstruction,
			GlobalInstructionProvider: llminternal.InstructionProvider(cfg.GlobalInstructionProvider),
		},
	}, nil
}

// Config of the LLMAgent.
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
	SubAgents []agent.Agent

	// Config holds generation parameters (temperature, top-p, etc.).
	Config *model.GenerateConfig
	// Model that is used by the agent.
	Model model.ModelAdapter
	// ModelID is the default model identifier for requests sent through this agent.
	ModelID string

	// Instruction is set for the LLM model guiding the agent's behavior.
	//
	// The string is treated as a template:
	//  - There can be placeholders like {key_name} that will be resolved by ADK
	//    at runtime using session state and context.
	//  - key_name must match "^[a-zA-Z_][a-zA-Z0-9_]*$", otherwise it will be
	//    treated as a literal.
	//  - {artifact.key_name} can be used to insert the text content of the
	//    artifact named key_name.
	//
	// If the state variable or artifact does not exist, the agent will raise an
	// error. If you want to ignore the error, you can append a ? to the
	// variable name as in {var?} to make it optional.
	//
	// If templating logic for {} chars is not desired, then InstructionProvider
	// should be used.
	Instruction string
	// InstructionProvider allows to create instructions dynamically based on
	// the agent context.
	//
	// It takes over the Instruction field if both are set.
	//
	// InstructionProvider does not automatically substitute values to {} and
	// treats them as just a raw char.
	// If you need to inject session state variables, use
	// util/instructionutil.InjectSessionState helper.
	InstructionProvider InstructionProvider

	// GlobalInstruction is the instruction for all agents in the entire
	// agent tree.
	//
	// The string is treated as a template:
	//  - There can be placeholders like {key_name} that will be resolved by ADK
	//    at runtime using session state and context.
	//  - key_name must match "^[a-zA-Z_][a-zA-Z0-9_]*$", otherwise it will be
	//    treated as a literal.
	//  - {artifact.key_name} can be used to insert the text content of the
	//    artifact named key_name.
	//
	// If the state variable or artifact does not exist, the agent will raise an
	// error. If you want to ignore the error, you can append a ? to the
	// variable name as in {var?} to make it optional.
	//
	// ONLY the GlobalInstruction in the root agent will take effect.
	//
	// For example: GlobalInstruction can make all agents have a stable identity
	// or personality.
	GlobalInstruction string
	// GlobalInstructionProvider allows to create global instructions
	// dynamically based on the agent context.
	//
	// It takes over the GlobalInstruction field if both are set.
	//
	// GlobalInstructionProvider does not automatically substitute values to {} and
	// treats them as just a raw char.
	// If you need to inject session state variables, use
	// util/instructionutil.InjectSessionState helper.
	GlobalInstructionProvider InstructionProvider

	// DisallowTransferToParent prevents transferring to parent agent if LLM
	// decides to.
	DisallowTransferToParent bool
	// DisallowTransferToPeers prevents transferring to peer agents.
	DisallowTransferToPeers bool

	// Whether to include contents (conversation history) in the model request.
	IncludeContents IncludeContents

	// InputSchema is the JSON Schema for the agent's input, when used as a tool.
	InputSchema map[string]any
	// OutputSchema is the JSON Schema for the agent's output.
	//
	// NOTE: when this is set, agent can only reply and cannot use any tools,
	// such as function tools, RAGs, agent transfer, etc.
	OutputSchema map[string]any
	// Tools available to the agent.
	Tools []tool.Tool
	// Toolsets will be used by llmagent to extract tools and pass to the
	// underlying LLM.
	Toolsets []tool.Toolset
}

// IncludeContents controls what parts of prior conversation history is received by llmagent.
type IncludeContents string

const (
	// IncludeContentsNone makes the llmagent operate solely on its current turn (latest user input + any following agent events).
	IncludeContentsNone IncludeContents = "none"
	// IncludeContentsDefault is enabled by default. The llmagent receives the relevant conversation history.
	IncludeContentsDefault IncludeContents = "default"
)

type llmAgent struct {
	agent.BaseAgent
	cfg llminternal.Config
}

func (a *llmAgent) AgentType() agent.Type {
	return agent.TypeLLMAgent
}

func (a *llmAgent) Run(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	return agent.WrapRun(ctx, a, a.run)
}

func (a *llmAgent) LLMConfig() *llminternal.Config {
	return &a.cfg
}

func (a *llmAgent) DeclaredTools() []tool.Tool {
	return a.cfg.DeclaredTools()
}

func (a *llmAgent) InputSchemaConfig() map[string]any {
	return a.cfg.InputSchemaConfig()
}

func (a *llmAgent) OutputSchemaConfig() map[string]any {
	return a.cfg.OutputSchemaConfig()
}

func (a *llmAgent) TransferToParentDisabled() bool {
	return a.cfg.TransferToParentDisabled()
}

func (a *llmAgent) run(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	prefix, prefixErr := llminternal.BuildPrefix(ctx, a, &a.cfg)
	// Extensions are per-invocation and don't mutate; filter each Kind once
	// so every step/tool call reuses the same slice.
	exts := ctx.RunConfig().Extensions
	turnExts := agent.AsTurnExtensions(exts)
	f := &llminternal.Flow{
		Model:     a.cfg.Model,
		Config:    &a.cfg,
		Agent:     a,
		Prefix:    prefix,
		Tree:      ctx.Tree(),
		ModelExts: agent.AsModelExtensions(exts),
		ToolExts:  agent.AsToolExtensions(exts),
	}

	return func(yield func(event.Event, error) bool) {
		if prefixErr != nil {
			yield(nil, fmt.Errorf("build llm prefix: %w", prefixErr))
			return
		}
		firstTurn := true
		turnIndex := 0
		// ResumeData is consumed exactly once: only the first step sees it; subsequent
		// steps run with ResumeData cleared.
		pendingResumeData := ctx.RunState().ResumeData
		for {
			stepRunState := agent.DeriveRunState(ctx.RunState(), func(state *agent.RunState) {
				state.ResumeData = pendingResumeData
			})
			pendingResumeData = nil
			stepCtx := ctx.WithRunState(stepRunState)
			if !yield(event.NewTurnStart(turnIndex), nil) {
				return
			}
			// BeforeTurn extension hook.
			beforeCtrl := &agent.TurnControl{}
			if err := runBeforeTurnExtensions(stepCtx, turnExts, beforeCtrl); err != nil {
				yield(nil, err)
				return
			}
			for _, msg := range beforeCtrl.Messages {
				if !yieldMessage(yield, msg, stepCtx) {
					return
				}
			}
			if beforeCtrl.EndInvocation {
				return
			}
			if firstTurn {
				firstTurn = false
				if userMsg := ctx.UserContent(); userMsg != nil {
					if !yield(event.NewMessageStart(userMsg), nil) {
						return
					}
					if !yield(event.NewMessageEnd(userMsg), nil) {
						return
					}
				}
			}

			stop := false
			aborted := false
			stepResult, err := f.Step(stepCtx, func(ev event.Event, err error) bool {
				if err != nil {
					yield(nil, err)
					aborted = true
					return false
				}
				if _, ok := ev.(*event.Interrupt); ok {
					stop = true
				}
				if !yield(ev, nil) {
					aborted = true
					return false
				}
				return true
			})
			if aborted {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			if stop || stepResult.Transferred {
				// Tool-triggered transfer already carries the target on the
				// tool_execution_end's Control.TransferToAgent — the runner reads
				// it from there. No extra AgentTransferRequest emit is needed.
				return
			}
			// AfterTurn extension hook.
			afterCtrl := &agent.TurnControl{}
			if err := runAfterTurnExtensions(stepCtx, turnExts, afterCtrl); err != nil {
				yield(nil, err)
				return
			}
			afterInjected := false
			for _, msg := range afterCtrl.Messages {
				if !yieldMessage(yield, msg, stepCtx) {
					return
				}
				afterInjected = true
			}
			if afterCtrl.EndInvocation {
				return
			}
			if !afterInjected && !stepResult.Continue && !afterCtrl.Continue {
				return
			}
			turnIndex++
		}
	}
}

// InstructionProvider computes an agent's instruction text. It is invoked
// exactly once per invocation, at prefix-build time, and the result is
// frozen into the invocation's static Prefix and reused across every step.
// This is what lets LLM-provider prompt caches (Anthropic cache_control,
// OpenAI prefix cache, Gemini context cache) hit on the system prompt, so
// providers must be deterministic with respect to any per-step state. The
// ReadonlyContext may be used for init-time reads (session metadata, user
// info) but not for anything that changes between steps.
//
// NOTE: when InstructionProvider is used, ADK will NOT inject session state
// placeholders into the instruction. You can use
// util/instructionutil.InjectSessionState() helper if this functionality is
// needed.
type InstructionProvider func(ctx agent.InvocationContext) (string, error)

// yieldMessage emits MessageStart and MessageEnd events for a message.
// WithAuthor is set explicitly so non-user roles (e.g. "tool") surface their
// role rather than being defaulted to the agent's own name.
func yieldMessage(yield func(event.Event, error) bool, msg *message.Message, _ agent.InvocationContext) bool {
	if !yield(event.NewMessageStart(msg, event.WithAuthor(string(msg.Role))), nil) {
		return false
	}
	return yield(event.NewMessageEnd(msg, event.WithAuthor(string(msg.Role))), nil)
}

func runBeforeTurnExtensions(ctx agent.InvocationContext, exts []agent.TurnExtension, ctrl *agent.TurnControl) error {
	return agent.ForEach(exts, func(ext agent.TurnExtension) (bool, error) {
		if err := ext.BeforeTurn(ctx, ctrl); err != nil {
			return true, fmt.Errorf("extension %q before turn: %w", ext.Name(), err)
		}
		return false, nil
	})
}

func runAfterTurnExtensions(ctx agent.InvocationContext, exts []agent.TurnExtension, ctrl *agent.TurnControl) error {
	return agent.ForEach(exts, func(ext agent.TurnExtension) (bool, error) {
		if err := ext.AfterTurn(ctx, ctrl); err != nil {
			return true, fmt.Errorf("extension %q after turn: %w", ext.Name(), err)
		}
		return false, nil
	})
}
