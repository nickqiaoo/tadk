// Package loopagent provides an agent that repeatedly runs its sub-agents for a
// specified number of iterations or until termination condition is met.
package loopagent

import (
	"fmt"
	"iter"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/workflowinternal"
)

// Config defines the configuration for a LoopAgent.
type Config struct {
	// Basic agent setup.
	AgentConfig agent.Config

	// If MaxIterations == 0, then LoopAgent runs indefinitely or until any
	// sub-agent escalates.
	MaxIterations uint
}

// New creates a LoopAgent.
//
// LoopAgent repeatedly runs its sub-agents in sequence for a specified number
// of iterations or until a termination condition is met.
//
// Use the LoopAgent when your workflow involves repetition or iterative
// refinement, such as like revising code.
func New(cfg Config) (agent.Agent, error) {
	if cfg.AgentConfig.Run != nil {
		return nil, fmt.Errorf("LoopAgent doesn't allow custom Run implementations")
	}

	base, err := agent.NewBaseAgent(cfg.AgentConfig.Name, cfg.AgentConfig.Description, cfg.AgentConfig.SubAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to create base agent: %w", err)
	}

	return &loopAgent{
		BaseAgent:     base,
		maxIterations: cfg.MaxIterations,
	}, nil
}

type loopAgent struct {
	agent.BaseAgent
	maxIterations uint
}

func (a *loopAgent) AgentType() agent.Type {
	return agent.TypeLoopAgent
}

func (a *loopAgent) Run(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	return agent.WrapRun(ctx, a, a.runImpl)
}

// loopState holds the state of the loop agent for resume.
type loopState struct {
	Iteration     uint `json:"iteration"`
	SubAgentIndex int  `json:"sub_agent_index"`
}

func (a *loopAgent) runImpl(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		subAgents := a.SubAgents()
		if len(subAgents) == 0 {
			return
		}

		// Check if this is a resume invocation. ResumeData is consumed exactly once,
		// at the sub-agent position recorded in the saved loopState.
		pendingResumeData := ctx.RunState().ResumeData
		startIteration := uint(0)
		startSubAgentIndex := 0
		if len(pendingResumeData) > 0 {
			state, found := workflowinternal.FindResumeState[loopState](ctx, workflowinternal.AgentAddress(ctx))
			if found {
				startIteration = state.Iteration
				startSubAgentIndex = state.SubAgentIndex
			}
		}

		iteration := startIteration
		interrupted := false
		defer func() {
			if !interrupted && ctx.Context().Err() == nil {
				_ = workflowinternal.ClearState(ctx)
			}
		}()
		for {
			shouldExit := false
			for i, subAgent := range subAgents {
				// Skip already completed sub-agents in the first iteration after resume
				if iteration == startIteration && i < startSubAgentIndex {
					continue
				}
				address := workflowinternal.SubAgentAddress(ctx, subAgent.Name())

				subResume := pendingResumeData
				pendingResumeData = nil
				subCtx := agent.DeriveInvocationContext(ctx, agent.InvocationContextOverrides{
					AgentName: subAgent.Name(),
					Address:   address,
					RunState:  workflowinternal.SubAgentRunState(ctx.RunState(), subResume),
				})

				for output, err := range subAgent.Run(subCtx) {
					if err != nil {
						yield(nil, err)
						return
					}

					// Check if sub-agent returned an interrupt
					if intr, ok := output.(*event.Interrupt); ok && intr.Interrupt != nil {
						composite, err := workflowinternal.SaveAndComposeInterrupt(
							ctx,
							loopState{Iteration: iteration, SubAgentIndex: i},
							fmt.Sprintf("%s interrupt", ctx.AgentName()),
							intr.Interrupt.ToSignals(),
						)
						if err != nil {
							yield(nil, err)
							return
						}
						interrupted = true
						yield(composite, nil)
						return
					}

					if !yield(output, nil) {
						return
					}

					if output != nil && output.Control().ExitToParent {
						shouldExit = true
					}
				}
				if shouldExit {
					return
				}
			}

			iteration++
			if a.maxIterations > 0 && iteration >= a.maxIterations {
				return
			}
		}
	}
}
