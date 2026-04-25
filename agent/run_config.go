package agent

import (
	"maps"

	"github.com/nickqiaoo/tadk/session"
)

// RunConfig is the stable, inheritable invocation configuration. It is shared
// by pointer across every InvocationContext within an invocation and MUST be
// treated as read-only by callers. To produce a modified variant, use
// DeriveRunConfig, which allocates a fresh copy before applying mutations.
type RunConfig struct {
	StreamingMode StreamingMode
	Extensions    []Extension
	TargetAgent   string
}

// RunState is the per-invocation runtime state derived or injected by the
// runtime. Like RunConfig it is shared by pointer and MUST be treated as
// read-only; use DeriveRunState to obtain a mutable copy.
type RunState struct {
	ResumeData        map[string]any
	Checkpoint        *session.Checkpoint
	AgentStateService session.Service
}

// cloneRunConfig returns a deep copy of src. A nil input yields a fresh empty
// RunConfig so callers can always assume a non-nil result.
func cloneRunConfig(src *RunConfig) *RunConfig {
	if src == nil {
		return &RunConfig{}
	}
	out := *src
	if len(src.Extensions) > 0 {
		out.Extensions = append([]Extension(nil), src.Extensions...)
	}
	return &out
}

// cloneRunState returns a deep copy of src. A nil input yields a fresh empty
// RunState so callers can always assume a non-nil result.
func cloneRunState(src *RunState) *RunState {
	if src == nil {
		return &RunState{}
	}
	out := *src
	if src.ResumeData != nil {
		out.ResumeData = maps.Clone(src.ResumeData)
	}
	return &out
}

// StreamingMode defines the streaming mode for agent execution.
type StreamingMode string

const (
	// StreamingModeNone indicates no streaming.
	StreamingModeNone StreamingMode = "none"
	// StreamingModeSSE enables server-sent events streaming, one-way, where
	// LLM response parts are streamed immediately as they are generated.
	StreamingModeSSE StreamingMode = "sse"
)

// DeriveRunConfig clones the parent config and applies the given mutation.
// The returned pointer is owned by the caller and safe to mutate.
func DeriveRunConfig(parent *RunConfig, mutate func(*RunConfig)) *RunConfig {
	out := cloneRunConfig(parent)
	if mutate != nil {
		mutate(out)
	}
	return out
}

// DeriveRunState clones the parent state and applies the given mutation.
// The returned pointer is owned by the caller and safe to mutate.
func DeriveRunState(parent *RunState, mutate func(*RunState)) *RunState {
	out := cloneRunState(parent)
	if mutate != nil {
		mutate(out)
	}
	return out
}
