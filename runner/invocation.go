package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

// invocation holds the mutable state for a single Runner.Run call. It owns the
// per-invocation InvocationContext (which is swapped on agent transfers) and
// precomputes runner-level extensions so each lifecycle hook does not re-filter
// them. Methods on invocation implement the orchestration phases (start,
// before-run hooks, agent iteration, transfer, finalize) that runOutputs used
// to inline.
type invocation struct {
	runner        *Runner
	ctx           context.Context
	storedSession session.Session
	invCtx        agent.InvocationContext

	runnerExts []agent.RunnerExtension

	resumeRun                bool
	wroteInterruptCheckpoint bool
}

// newInvocation loads the session, splits run options into cfg/state, derives
// the initial InvocationContext, and chooses the starting agent (root or
// TargetAgent). It does not persist the initial user message; callers must do
// that after inspecting the returned invocation.
func (r *Runner) newInvocation(ctx context.Context, userID, sessionID string, userMsg *message.Message, opts agent.RunOptions) (*invocation, agent.Agent, error) {
	resp, err := r.sessionService.Get(ctx, &session.GetRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, nil, &FatalError{Cause: fmt.Errorf("session get failed: %w", err)}
	}
	storedSession := resp.Session

	runCfg, runState := opts.Split()
	if len(r.extensions) > 0 && len(runCfg.Extensions) == 0 {
		runCfg.Extensions = append([]agent.Extension(nil), r.extensions...)
	}
	if runState.AgentStateService == nil {
		runState.AgentStateService = r.sessionService
	}

	resumeRun := len(opts.ResumeData) > 0
	if resumeRun {
		runState.Checkpoint, err = r.checkpointManager.load(ctx, storedSession)
		if err != nil {
			return nil, nil, &FatalError{Cause: fmt.Errorf("checkpoint load failed: %w", err)}
		}
	}

	agentToRun := r.rootAgent
	if !resumeRun && runCfg.TargetAgent != "" {
		agentToRun = r.tree.FindByName(runCfg.TargetAgent)
		if agentToRun == nil {
			return nil, nil, &FatalError{Cause: fmt.Errorf("target agent %q not found", runCfg.TargetAgent)}
		}
	}

	invCtx := agent.NewInvocationContext(ctx, agent.InvocationContextParams{
		Session:     storedSession,
		Address:     agentToRun.Name(),
		AgentName:   agentToRun.Name(),
		UserContent: userMsg,
		RunConfig:   runCfg,
		RunState:    runState,
		Tree:        r.tree,
	})

	inv := &invocation{
		runner:        r,
		ctx:           ctx,
		storedSession: storedSession,
		invCtx:        invCtx,
		runnerExts:    agent.AsRunnerExtensions(runCfg.Extensions),
		resumeRun:     resumeRun,
	}
	return inv, agentToRun, nil
}

// persistUserMessage writes the initial user message to the session log.
func (inv *invocation) persistUserMessage(msg *message.Message) error {
	if msg == nil {
		return nil
	}
	if err := inv.runner.persistor.appendMessage(inv.ctx, inv.storedSession, inv.invCtx, msg); err != nil {
		return &FatalError{Cause: fmt.Errorf("persist message failed: %w", err)}
	}
	return nil
}

// runAgent runs a single agent through the full before/stream/after pipeline
// and returns the next agent to run on transfer, or nil to stop. stop=true
// means yield signaled abandonment and the outer iterator should return.
func (inv *invocation) runAgent(agentToRun agent.Agent, yield func(event.Event, error) bool) (next agent.Agent, stop bool, err error) {
	runCtrl := &agent.RunControl{}
	short, err := inv.runBefore(agentToRun, runCtrl)
	if err != nil {
		return nil, false, err
	}
	if short != nil {
		stop, err := inv.handleShortCircuit(short, yield)
		if err != nil {
			return nil, false, err
		}
		if stop {
			return nil, true, nil
		}
		if err := inv.runAfter(runCtrl); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}

	transferTo, transferBranch, transferInvID, stop, err := inv.streamAgentEvents(agentToRun, yield)
	if stop || err != nil {
		return nil, stop, err
	}

	if err := inv.runAfter(runCtrl); err != nil {
		return nil, false, err
	}

	if transferTo == "" {
		return nil, false, nil
	}
	nextAgent := inv.runner.tree.FindByName(transferTo)
	if nextAgent == nil {
		return nil, false, fmt.Errorf("target agent %q not found", transferTo)
	}
	inv.invCtx = agent.DeriveInvocationContext(inv.invCtx, agent.InvocationContextOverrides{
		Context:      inv.ctx,
		AgentName:    transferTo,
		Address:      transferTo,
		Branch:       &transferBranch,
		InvocationID: &transferInvID,
	})
	return nextAgent, false, nil
}

// runBefore iterates BeforeRun hooks; returns a short-circuit event if any hook
// returned a non-nil message.
func (inv *invocation) runBefore(agentToRun agent.Agent, ctrl *agent.RunControl) (event.Event, error) {
	var result event.Event
	err := agent.ForEach(inv.runnerExts, func(ext agent.RunnerExtension) (bool, error) {
		msg, err := ext.BeforeRun(inv.invCtx, ctrl)
		if err != nil {
			return true, fmt.Errorf("extension %q before run failed: %w", ext.Name(), err)
		}
		if msg != nil {
			result = event.NewMessageEnd(msg, event.WithAuthor(agentToRun.Name()))
			return true, nil
		}
		return false, nil
	})
	return result, err
}

// runOnEvent pipes an event through all OnEvent hooks. A nil return means the
// event was filtered out and should not be yielded.
func (inv *invocation) runOnEvent(ev event.Event) (event.Event, error) {
	if ev == nil {
		return ev, nil
	}
	err := agent.ForEach(inv.runnerExts, func(ext agent.RunnerExtension) (bool, error) {
		var err error
		ev, err = ext.OnEvent(inv.invCtx, ev)
		if err != nil {
			return true, fmt.Errorf("extension %q on event failed: %w", ext.Name(), err)
		}
		if ev == nil {
			return true, nil
		}
		return false, nil
	})
	return ev, err
}

// runAfter iterates AfterRun hooks.
func (inv *invocation) runAfter(ctrl *agent.RunControl) error {
	return agent.ForEach(inv.runnerExts, func(ext agent.RunnerExtension) (bool, error) {
		if err := ext.AfterRun(inv.invCtx, ctrl); err != nil {
			return true, fmt.Errorf("extension %q after run failed: %w", ext.Name(), err)
		}
		return false, nil
	})
}

// handleShortCircuit processes a synthetic event emitted by a BeforeRun hook:
// it runs OnEvent, persists, checkpoints on interrupt, and yields. Returns
// stop=true if the consumer abandoned iteration.
func (inv *invocation) handleShortCircuit(ev event.Event, yield func(event.Event, error) bool) (bool, error) {
	ev, err := inv.runOnEvent(ev)
	if err != nil {
		return false, err
	}
	if ev == nil {
		return false, nil
	}
	if err := inv.runner.persistor.persistEvent(inv.ctx, inv.storedSession, ev); err != nil {
		return false, err
	}
	if interrupt, ok := ev.(*event.Interrupt); ok {
		inv.wroteInterruptCheckpoint = true
		if err := inv.runner.checkpointManager.save(inv.ctx, inv.storedSession, inv.invCtx, interrupt.Interrupt, nil); err != nil {
			return false, err
		}
	}
	if !yield(ev, nil) {
		return true, nil
	}
	return false, nil
}

// streamAgentEvents iterates the agent's event stream. It runs each event
// through OnEvent, detects transfer requests, persists, saves interrupt
// checkpoints, and yields. Returns stop=true when the consumer signaled
// abandonment.
func (inv *invocation) streamAgentEvents(agentToRun agent.Agent, yield func(event.Event, error) bool) (transferTo, transferBranch, transferInvID string, stop bool, err error) {
	for ev, evErr := range agentToRun.Run(inv.invCtx) {
		if evErr != nil {
			if errors.Is(evErr, ErrFatal) {
				return "", "", "", false, evErr
			}
			if !yield(nil, evErr) {
				return "", "", "", true, nil
			}
			continue
		}
		ev, hookErr := inv.runOnEvent(ev)
		if hookErr != nil {
			return "", "", "", false, hookErr
		}
		if ev == nil {
			continue
		}
		// Transfer is a control signal carried on Control.TransferToAgent;
		// any event (AgentTransferRequest from a custom agent, tool_execution_end
		// from a tool-triggered transfer, etc.) can express transfer intent the
		// same way. Last writer wins so a later event can override an earlier one.
		if target := ev.Control().TransferToAgent; target != "" {
			transferTo = target
			transferBranch = inv.invCtx.Branch()
			transferInvID = inv.invCtx.InvocationID()
			meta := ev.Meta()
			if meta.Branch != "" {
				transferBranch = meta.Branch
			}
			if meta.InvocationID != "" {
				transferInvID = meta.InvocationID
			}
		}
		if persistErr := inv.runner.persistor.persistEvent(inv.ctx, inv.storedSession, ev); persistErr != nil {
			return "", "", "", false, persistErr
		}
		if interrupt, ok := ev.(*event.Interrupt); ok {
			inv.wroteInterruptCheckpoint = true
			if saveErr := inv.runner.checkpointManager.save(inv.ctx, inv.storedSession, inv.invCtx, interrupt.Interrupt, nil); saveErr != nil {
				return "", "", "", false, saveErr
			}
		}
		if !yield(ev, nil) {
			return "", "", "", true, nil
		}
	}
	return transferTo, transferBranch, transferInvID, false, nil
}

// finalize clears the resume checkpoint if the invocation completed without
// emitting a new interrupt.
func (inv *invocation) finalize() error {
	if inv.resumeRun && !inv.wroteInterruptCheckpoint {
		return inv.runner.checkpointManager.clear(inv.ctx, inv.storedSession)
	}
	return nil
}
