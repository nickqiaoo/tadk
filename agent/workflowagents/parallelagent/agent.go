// Package parallelagent provides an agent that runs its sub-agents in parallel.
package parallelagent

import (
	"fmt"
	"iter"

	"golang.org/x/sync/errgroup"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/workflowinternal"
	"github.com/nickqiaoo/tadk/resume"
)

// Config defines the configuration for a ParallelAgent.
type Config struct {
	// Basic agent setup.
	AgentConfig agent.Config
}

// New creates a ParallelAgent.
//
// Parallel agent runs its sub-agents in parallel in isolated manner.
//
// This approach is beneficial for scenarios requiring multiple perspectives or
// attempts on a single task, such as:
// - Running different algorithms simultaneously.
// - Generating multiple responses for review by a subsequent evaluation agent.
func New(cfg Config) (agent.Agent, error) {
	if cfg.AgentConfig.Run != nil {
		return nil, fmt.Errorf("ParallelAgent doesn't allow custom Run implementations")
	}

	base, err := agent.NewBaseAgent(cfg.AgentConfig.Name, cfg.AgentConfig.Description, cfg.AgentConfig.SubAgents)
	if err != nil {
		return nil, err
	}
	return &parallelAgent{BaseAgent: base}, nil
}

type parallelAgent struct {
	agent.BaseAgent
}

func (a *parallelAgent) AgentType() agent.Type {
	return agent.TypeParallelAgent
}

func (a *parallelAgent) Run(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	return agent.WrapRun(ctx, a, a.runImpl)
}

func (a *parallelAgent) runImpl(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
	subAgents := a.SubAgents()
	// On resume, every still-running sub-agent sees the same ResumeData payload:
	// each sub-agent keyed its own interrupts, so only the matching entries fire.
	resumeData := ctx.RunState().ResumeData
	isResume := len(resumeData) > 0

	// ========== 1. Check if this is a resume invocation ==========
	var completedAgents map[string]struct{}

	if isResume {
		state, found := workflowinternal.FindResumeState[parallelState](ctx, workflowinternal.AgentAddress(ctx))
		if found {
			completedAgents = make(map[string]struct{})
			for _, name := range state.CompletedSubAgents {
				completedAgents[name] = struct{}{}
			}
		}
	}

	// ========== 2. Start parallel execution ==========
	var (
		errGroup, errGroupCtx = errgroup.WithContext(ctx.Context())
		doneChan              = make(chan bool)
		resultsChan           = make(chan result)
	)

	for _, sa := range subAgents {
		// Skip already completed sub-agents
		if _, ok := completedAgents[sa.Name()]; ok {
			continue
		}

		branch := makeBranch(ctx, sa.Name())
		address := workflowinternal.SubAgentAddress(ctx, sa.Name())
		subAgent := sa

		subState := prepareSubAgentRunState(ctx, resumeData, isResume)

		errGroup.Go(func() error {
			subCtx := agent.DeriveInvocationContext(ctx, agent.InvocationContextOverrides{
				Context:   errGroupCtx,
				AgentName: subAgent.Name(),
				Address:   address,
				Branch:    &branch,
				RunState:  subState,
			})

			if err := runSubAgent(subCtx, subAgent, resultsChan, doneChan); err != nil {
				return fmt.Errorf("failed to run sub-agent %q: %w", subAgent.Name(), err)
			}

			return nil
		})
	}

	go func() {
		// errGroup.Wait is intentionally ignored: sub-agent errors reach the
		// consumer through resultsChan (runSubAgent sends {err: ...} before
		// returning). The only case where an error is returned without being
		// emitted is when doneChan is closed first — which means the consumer
		// has already abandoned iteration and is no longer listening.
		_ = errGroup.Wait()
		close(resultsChan)
	}()

	// ========== 3. Collect results and handle interrupts ==========
	return func(yield func(event.Event, error) bool) {
		defer close(doneChan)
		interrupted := false
		defer func() {
			if !interrupted && ctx.Context().Err() == nil {
				_ = workflowinternal.ClearState(ctx)
			}
		}()

		var (
			completed  []string                  // Completed sub-agents in this run
			interrupts []*resume.InterruptSignal // Collected interrupt signals
		)

		for res := range resultsChan {
			if res.err != nil {
				if !yield(nil, res.err) {
					return
				}
				continue
			}

			// Check if this is an interrupt event
			if intr, ok := res.output.(*event.Interrupt); ok {
				sigs := intr.Interrupt.ToSignals()
				interrupts = append(interrupts, sigs...)
				// Don't yield interrupt event now, wait for composition
				continue
			}

			// Track completed sub-agents (by checking final response)
			if res.output != nil && event.IsFinalResponse(res.output) && res.subAgent != "" {
				completed = append(completed, res.subAgent)
			}

			// Yield normal events
			if !yield(res.output, nil) {
				return
			}
		}

		// ========== 4. Compose interrupt if any ==========
		if len(interrupts) > 0 {
			// Calculate all completed sub-agents (including previously completed from resume)
			allCompleted := make([]string, 0, len(completedAgents)+len(completed))
			for name := range completedAgents {
				allCompleted = append(allCompleted, name)
			}
			allCompleted = append(allCompleted, completed...)

			composite, err := workflowinternal.SaveAndComposeInterrupt(
				ctx,
				parallelState{CompletedSubAgents: allCompleted},
				"parallel agent interrupt",
				interrupts,
			)
			if err != nil {
				yield(nil, err)
				return
			}
			interrupted = true
			yield(composite, nil)
		}
	}
}

func runSubAgent(ctx agent.InvocationContext, ag agent.Agent, results chan<- result, done <-chan bool) error {
	agentName := ag.Name()
	for output, err := range ag.Run(ctx) {
		select {
		case <-done:
			return nil
		case <-ctx.Context().Done():
			select {
			case <-done:
			case results <- result{
				err:      ctx.Context().Err(),
				subAgent: agentName,
			}:
			}
			return ctx.Context().Err()
		case results <- result{
			output:   output,
			err:      err,
			subAgent: agentName,
		}:
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type result struct {
	output   event.Event
	err      error
	subAgent string
}

// parallelState holds the state of the parallel agent for resume.
type parallelState struct {
	CompletedSubAgents []string `json:"completed_sub_agents"`
}

// makeBranch creates the branch for a sub-agent.
func makeBranch(ctx agent.InvocationContext, subAgentName string) string {
	if ctx.Branch() != "" {
		return fmt.Sprintf("%s.%s", ctx.Branch(), subAgentName)
	}
	return fmt.Sprintf("%s.%s", ctx.AgentName(), subAgentName)
}

// prepareSubAgentRunState prepares RunState for a sub-agent, scoping ResumeData to
// resume invocations only.
func prepareSubAgentRunState(ctx agent.InvocationContext, resumeData map[string]any, isResume bool) *agent.RunState {
	if !isResume {
		resumeData = nil
	}
	return workflowinternal.SubAgentRunState(ctx.RunState(), resumeData)
}
