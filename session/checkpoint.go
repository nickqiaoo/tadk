package session

// CheckpointStatus describes the resumability state of a session.
type CheckpointStatus string

const (
	CheckpointStatusRunning     CheckpointStatus = "running"
	CheckpointStatusInterrupted CheckpointStatus = "interrupted"
	CheckpointStatusCompleted   CheckpointStatus = "completed"
)

// Checkpoint stores durable runtime control state for a session.
//
// Entries are still the append-only audit ledger. Checkpoint is the current
// routing/index state used to resume an active interruption without scanning
// the entire entry log.
type Checkpoint struct {
	Version           int                          `json:"version"`
	Revision          int64                        `json:"revision"`
	Status            CheckpointStatus             `json:"status"`
	Branch            string                       `json:"branch,omitempty"`
	InvocationID      string                       `json:"invocationId,omitempty"`
	EntryID           string                       `json:"entryId,omitempty"`
	WorkflowID        string                       `json:"workflowId,omitempty"`
	PendingInterrupts map[string]*PendingInterrupt `json:"pendingInterrupts,omitempty"`
	UpdatedAt         string                       `json:"updatedAt,omitempty"`
}

// PendingInterrupt points to the agent/tool state needed to resume an
// interruption.
type PendingInterrupt struct {
	ID           string         `json:"id"`
	Address      string         `json:"address,omitempty"`
	AgentAddress string         `json:"agentAddress,omitempty"`
	ToolCallID   string         `json:"toolCallId,omitempty"`
	ToolName     string         `json:"toolName,omitempty"`
	ToolArgs     map[string]any `json:"toolArgs,omitempty"`
	State        any            `json:"state,omitempty"`
	Info         any            `json:"info,omitempty"`
	CreatedAt    string         `json:"createdAt,omitempty"`
}

// CheckpointRequest identifies a session checkpoint.
type CheckpointRequest struct {
	AppName   string
	UserID    string
	SessionID string
}

// SaveCheckpointRequest writes a checkpoint. If ExpectedRevision is non-zero,
// implementations should reject writes when the stored revision differs.
type SaveCheckpointRequest struct {
	CheckpointRequest
	Checkpoint       *Checkpoint
	ExpectedRevision int64
}
