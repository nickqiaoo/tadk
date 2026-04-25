package session

import "testing"

func TestInMemoryService(t *testing.T) {
	ctx := t.Context()
	svc := InMemoryService()

	if _, err := svc.Create(ctx, &CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	}); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if err := svc.SaveCheckpoint(ctx, &SaveCheckpointRequest{
		CheckpointRequest: CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"},
		Checkpoint: &Checkpoint{
			Status:     CheckpointStatusInterrupted,
			WorkflowID: "workflow-1",
			PendingInterrupts: map[string]*PendingInterrupt{
				"interrupt-1": {
					ID:           "interrupt-1",
					AgentAddress: "parallel_agent.sub_agent_2",
					ToolCallID:   "call-1",
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveCheckpoint(): %v", err)
	}

	checkpoint, err := svc.GetCheckpoint(ctx, &CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"})
	if err != nil {
		t.Fatalf("GetCheckpoint(): %v", err)
	}
	if checkpoint == nil {
		t.Fatal("checkpoint is nil")
	}
	if checkpoint.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", checkpoint.Revision)
	}
	if checkpoint.WorkflowID != "workflow-1" {
		t.Fatalf("WorkflowID = %q, want workflow-1", checkpoint.WorkflowID)
	}

	if err := svc.ClearCheckpoint(ctx, &CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"}); err != nil {
		t.Fatalf("ClearCheckpoint(): %v", err)
	}
	checkpoint, err = svc.GetCheckpoint(ctx, &CheckpointRequest{AppName: "app", UserID: "user", SessionID: "sess"})
	if err != nil {
		t.Fatalf("GetCheckpoint() after clear: %v", err)
	}
	if checkpoint != nil {
		t.Fatal("checkpoint after clear is non-nil")
	}
}
