// Package workflowinternal provides shared utilities for workflow agents.
package workflowinternal

import (
	"encoding/json"
	"fmt"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/session"
)

// AgentAddress returns the address of the current agent.
func AgentAddress(ctx agent.InvocationContext) string {
	return ctx.Address()
}

// SubAgentAddress returns the address for a sub-agent.
func SubAgentAddress(ctx agent.InvocationContext, subAgentName string) string {
	return fmt.Sprintf("%s.%s", ctx.Address(), subAgentName)
}

// FindResumeState loads the workflow state if the current agent has a pending interrupt.
func FindResumeState[T any](ctx agent.InvocationContext, address string) (T, bool) {
	var zero T
	if state := ctx.RunState(); state.Checkpoint != nil {
		for _, pending := range state.Checkpoint.PendingInterrupts {
			if pending == nil || pending.Address != address {
				continue
			}
			state, err := LoadState[T](ctx)
			if err != nil {
				return zero, false
			}
			return state, true
		}
	}
	return zero, false
}

// LoadState loads the agent state from the session store and unmarshals it into T.
func LoadState[T any](ctx agent.InvocationContext) (T, error) {
	var result T
	state := ctx.RunState()
	if state.AgentStateService == nil {
		return result, nil
	}
	agentState, err := state.AgentStateService.GetAgentState(ctx.Context(), &session.AgentStateRequest{
		AppName:      ctx.Session().AppName(),
		UserID:       ctx.Session().UserID(),
		SessionID:    ctx.Session().ID(),
		AgentAddress: ctx.Address(),
	})
	if err != nil || agentState == nil || agentState.Data == nil {
		return result, err
	}
	// Marshal/unmarshal via JSON to convert map[string]any -> T reliably.
	b, err := json.Marshal(agentState.Data)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return result, err
	}
	return result, nil
}

// SaveState marshals the given state to JSON and saves it to the session store.
func SaveState[T any](ctx agent.InvocationContext, state T) error {
	runState := ctx.RunState()
	if runState.AgentStateService == nil {
		return nil
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	return runState.AgentStateService.SaveAgentState(ctx.Context(), &session.SaveAgentStateRequest{
		AgentStateRequest: session.AgentStateRequest{
			AppName:      ctx.Session().AppName(),
			UserID:       ctx.Session().UserID(),
			SessionID:    ctx.Session().ID(),
			AgentAddress: ctx.Address(),
		},
		State: &session.AgentState{
			AgentAddress: ctx.Address(),
			Data:         data,
		},
	})
}

// SubAgentRunState scopes ResumeData to a single sub-agent invocation.
// Pass nil once resume data has been consumed so downstream invocations run fresh.
func SubAgentRunState(parent *agent.RunState, resumeData map[string]any) *agent.RunState {
	return agent.DeriveRunState(parent, func(state *agent.RunState) {
		state.ResumeData = resumeData
	})
}

// SaveAndComposeInterrupt persists `state` then builds a composite interrupt event
// aggregating the provided sub-agent signals under the current agent address.
func SaveAndComposeInterrupt[T any](ctx agent.InvocationContext, state T, description string, subSignals []*resume.InterruptSignal) (event.Event, error) {
	if err := SaveState(ctx, state); err != nil {
		return nil, err
	}
	composite := resume.CompositeInterrupt(AgentAddress(ctx), description, nil, subSignals...)
	return event.NewInterrupt(composite.ToInterruptData()), nil
}

// ClearState removes the agent state from the session store.
func ClearState(ctx agent.InvocationContext) error {
	state := ctx.RunState()
	if state.AgentStateService == nil {
		return nil
	}
	return state.AgentStateService.ClearAgentState(ctx.Context(), &session.AgentStateRequest{
		AppName:      ctx.Session().AppName(),
		UserID:       ctx.Session().UserID(),
		SessionID:    ctx.Session().ID(),
		AgentAddress: ctx.Address(),
	})
}
