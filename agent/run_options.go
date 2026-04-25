package agent

import "github.com/nickqiaoo/tadk/session"

// RunOptions is the flat DTO accepted at the public Runner.Run boundary.
// Internally the runtime splits it into RunConfig + RunState and passes
// them through the invocation context by pointer; RunOptions is never used
// as an internal value.
type RunOptions struct {
	// StreamingMode defines the streaming mode for an agent.
	StreamingMode StreamingMode
	// Extensions are runtime hooks applied at runner/model/tool boundaries.
	Extensions []Extension
	// ResumeData carries resume payload keyed by interrupt IDs.
	// When set, Run executes a resume flow instead of a new invocation.
	ResumeData map[string]any
	// AgentStateService stores agent-local durable state outside transcript
	// history and session checkpoints.
	AgentStateService session.Service

	// TargetAgent explicitly names the agent that should handle this run.
	// If empty, the root agent is used.
	TargetAgent string
}

// Split converts public RunOptions into the internal (RunConfig, RunState)
// pair. Slices and maps are deep-copied once, so the returned pointers are
// fully owned by the runtime and safe to share across derived contexts
// without further cloning.
func (o RunOptions) Split() (*RunConfig, *RunState) {
	cfg := &RunConfig{
		StreamingMode: o.StreamingMode,
		TargetAgent:   o.TargetAgent,
	}
	if len(o.Extensions) > 0 {
		cfg.Extensions = append([]Extension(nil), o.Extensions...)
	}
	state := &RunState{
		AgentStateService: o.AgentStateService,
	}
	if o.ResumeData != nil {
		state.ResumeData = make(map[string]any, len(o.ResumeData))
		for k, v := range o.ResumeData {
			state.ResumeData[k] = v
		}
	}
	return cfg, state
}
