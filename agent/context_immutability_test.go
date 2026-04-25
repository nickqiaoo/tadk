package agent

import (
	"context"
	"testing"
)

// Getters return the same read-only pointer shared across derived contexts;
// callers must not mutate the returned RunConfig/RunState. This test asserts
// pointer identity to guard against accidental reintroduction of defensive
// copies on the hot path.
func TestInvocationContextSharesRunPointers(t *testing.T) {
	t.Parallel()

	cfg := &RunConfig{
		StreamingMode: StreamingModeSSE,
		Extensions:    []Extension{DefaultExtension{}},
		TargetAgent:   "target",
	}
	state := &RunState{
		ResumeData: map[string]any{"interrupt-1": "approved"},
	}

	ctx := NewInvocationContext(context.Background(), InvocationContextParams{
		RunConfig: cfg,
		RunState:  state,
	})

	if ctx.RunConfig() != cfg {
		t.Fatalf("RunConfig() returned a different pointer; getter must not clone")
	}
	if ctx.RunState() != state {
		t.Fatalf("RunState() returned a different pointer; getter must not clone")
	}

	// WithContext must preserve the shared RunConfig/RunState pointers.
	derived := ctx.WithContext(context.Background())
	if derived.RunConfig() != cfg {
		t.Fatalf("WithContext altered RunConfig pointer identity")
	}
	if derived.RunState() != state {
		t.Fatalf("WithContext altered RunState pointer identity")
	}
}

// DeriveRunConfig / DeriveRunState must return freshly owned pointers so
// mutations do not leak back into the parent context.
func TestDeriveIsolatesMutations(t *testing.T) {
	t.Parallel()

	parentCfg := &RunConfig{StreamingMode: StreamingModeSSE}
	parentState := &RunState{ResumeData: map[string]any{"k": "v"}}

	childCfg := DeriveRunConfig(parentCfg, func(c *RunConfig) {
		c.StreamingMode = StreamingModeNone
	})
	if childCfg == parentCfg {
		t.Fatal("DeriveRunConfig returned the parent pointer")
	}
	if parentCfg.StreamingMode != StreamingModeSSE {
		t.Fatalf("parent RunConfig mutated: %q", parentCfg.StreamingMode)
	}

	childState := DeriveRunState(parentState, func(s *RunState) {
		s.ResumeData["k"] = "mutated"
	})
	if childState == parentState {
		t.Fatal("DeriveRunState returned the parent pointer")
	}
	if parentState.ResumeData["k"] != "v" {
		t.Fatalf("parent ResumeData mutated: %v", parentState.ResumeData["k"])
	}
}
