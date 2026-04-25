package agent

import "context"

// InvocationContextOverrides describes the fields that should differ from a
// parent invocation context when starting a derived agent call. RunConfig and
// RunState, when non-nil, override their respective parent values; ownership
// of the pointers transfers to the new context.
type InvocationContextOverrides struct {
	Context      context.Context
	AgentName    string
	Address      string
	Branch       *string
	InvocationID *string
	RunConfig    *RunConfig
	RunState     *RunState
}

// DeriveInvocationContext creates a child invocation context by inheriting the
// stable invocation-scoped fields from parent and overriding only the fields
// explicitly provided by the caller. Unless overridden, the child shares
// parent's RunConfig and RunState pointers directly (zero copy).
func DeriveInvocationContext(parent InvocationContext, overrides InvocationContextOverrides) InvocationContext {
	ctx := overrides.Context
	params := InvocationContextParams{}

	if parent != nil {
		if ctx == nil {
			ctx = parent.Context()
		}
		params = InvocationContextParams{
			Session:      parent.Session(),
			Branch:       parent.Branch(),
			Address:      parent.Address(),
			AgentName:    parent.AgentName(),
			UserContent:  parent.UserContent(),
			RunConfig:    parent.RunConfig(),
			RunState:     parent.RunState(),
			InvocationID: parent.InvocationID(),
			Tree:         parent.Tree(),
		}
	}

	if overrides.AgentName != "" {
		params.AgentName = overrides.AgentName
	}
	if overrides.Address != "" {
		params.Address = overrides.Address
	}
	if overrides.Branch != nil {
		params.Branch = *overrides.Branch
	}
	if overrides.InvocationID != nil {
		params.InvocationID = *overrides.InvocationID
	}
	if overrides.RunConfig != nil {
		params.RunConfig = overrides.RunConfig
	}
	if overrides.RunState != nil {
		params.RunState = overrides.RunState
	}

	return NewInvocationContext(ctx, params)
}
