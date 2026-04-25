package agent

// Type identifies the concrete kind of an agent implementation.
type Type string

const (
	TypeLLMAgent        Type = "LLMAgent"
	TypeLoopAgent       Type = "LoopAgent"
	TypeSequentialAgent Type = "SequentialAgent"
	TypeParallelAgent   Type = "ParallelAgent"
	TypeCustomAgent     Type = "CustomAgent"
)

// Typed is implemented by any value that exposes an agent type.
type Typed interface {
	AgentType() Type
}

// TypeOf returns the agent type for a value.
// If the value does not implement Typed, it returns TypeCustomAgent.
func TypeOf(a any) Type {
	typed, ok := a.(Typed)
	if !ok {
		return TypeCustomAgent
	}
	return typed.AgentType()
}
