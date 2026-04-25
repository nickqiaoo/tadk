package session

// AgentState stores durable agent-local state outside transcript and session
// checkpoint. It is primarily used by local multi-agent execution.
type AgentState struct {
	Version      int            `json:"version"`
	AgentAddress string         `json:"agentAddress"`
	Data         map[string]any `json:"data,omitempty"`
	UpdatedAt    string         `json:"updatedAt,omitempty"`
}

// AgentStateRequest identifies agent-local state under a session.
type AgentStateRequest struct {
	AppName      string
	UserID       string
	SessionID    string
	AgentAddress string
}

// SaveAgentStateRequest writes an agent-local state snapshot.
type SaveAgentStateRequest struct {
	AgentStateRequest
	State *AgentState
}
