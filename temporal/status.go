package temporal

// Status describes the logical execution state of a durable tool workflow.
//
// StatusInterrupted is a workflow-internal paused state: the Temporal workflow
// is still running and is blocked waiting for a resume/cancel signal. It is
// distinct from the Temporal-native terminal states exposed by the server.
type Status string

const (
	StatusRunning     Status = "running"
	StatusInterrupted Status = "interrupted"
	StatusCompleted   Status = "completed"
	StatusCancelled   Status = "cancelled"
	StatusFailed      Status = "failed"
)
