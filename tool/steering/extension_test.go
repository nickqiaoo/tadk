package steering

import (
	"sync"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/message"
)

func TestSteerDrainsInAfterTurn(t *testing.T) {
	ext := New()
	ext.Steer(message.NewUserMessage("steer1"), message.NewUserMessage("steer2"))

	ctrl := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl); err != nil {
		t.Fatal(err)
	}

	if len(ctrl.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(ctrl.Messages))
	}
	if ctrl.Continue {
		t.Error("Continue should be false for steer messages")
	}

	// Second call should return nothing.
	ctrl2 := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl2); err != nil {
		t.Fatal(err)
	}
	if len(ctrl2.Messages) != 0 {
		t.Fatalf("got %d messages after drain, want 0", len(ctrl2.Messages))
	}
}

func TestFollowUpSetsContinue(t *testing.T) {
	ext := New()
	ext.FollowUp(message.NewUserMessage("followup"))

	ctrl := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl); err != nil {
		t.Fatal(err)
	}

	if len(ctrl.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(ctrl.Messages))
	}
	if !ctrl.Continue {
		t.Error("Continue should be true for follow-up messages")
	}
}

func TestSteerPriorityOverFollowUp(t *testing.T) {
	ext := New()
	ext.Steer(message.NewUserMessage("steer"))
	ext.FollowUp(message.NewUserMessage("followup"))

	// First drain: only steer messages.
	ctrl := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(ctrl.Messages))
	}
	if ctrl.Continue {
		t.Error("Continue should be false when steer messages are present")
	}

	// Second drain: follow-up messages.
	ctrl2 := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl2); err != nil {
		t.Fatal(err)
	}
	if len(ctrl2.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(ctrl2.Messages))
	}
	if !ctrl2.Continue {
		t.Error("Continue should be true for follow-up")
	}
}

func TestEmptyQueueNoOp(t *testing.T) {
	ext := New()
	ctrl := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.Messages) != 0 {
		t.Fatalf("got %d messages, want 0", len(ctrl.Messages))
	}
	if ctrl.Continue {
		t.Error("Continue should be false for empty queue")
	}
}

func TestConcurrentSteer(t *testing.T) {
	ext := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ext.Steer(message.NewUserMessage("msg"))
		}()
	}
	wg.Wait()

	ctrl := &agent.TurnControl{}
	if err := ext.AfterTurn(nil, ctrl); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.Messages) != 100 {
		t.Fatalf("got %d messages, want 100", len(ctrl.Messages))
	}
}

func TestName(t *testing.T) {
	ext := New()
	if ext.Name() != "steering" {
		t.Fatalf("got name %q, want %q", ext.Name(), "steering")
	}
}
