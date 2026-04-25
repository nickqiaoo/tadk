package runner

import (
	"context"
	"iter"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

func TestRunnerTransferPreservesInvocationIdentityAndBranch(t *testing.T) {
	t.Parallel()

	sourceInvocationID := ""
	targetBranch := ""
	targetInvocationID := ""

	target := mustNewAgent(t, agent.Config{
		Name: "target",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				targetBranch = ctx.Branch()
				targetInvocationID = ctx.InvocationID()
				yield(event.NewMessageEnd(
					message.NewAssistantMessage("transferred"),
					event.WithAuthor("target"),
					event.WithBranch(ctx.Branch()),
					event.WithInvocationID(ctx.InvocationID()),
				), nil)
			}
		},
	})

	source := mustNewAgent(t, agent.Config{
		Name:      "source",
		SubAgents: []agent.Agent{target},
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				sourceInvocationID = ctx.InvocationID()
				yield(event.NewAgentTransferRequest("target",
					event.WithBranch("branchful.path"),
					event.WithInvocationID(ctx.InvocationID()),
				), nil)
			}
		},
	})

	store := session.InMemoryService()
	created, err := store.Create(context.Background(), &session.CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	r, err := New(Config{
		AppName:        "app",
		Agent:          source,
		SessionService: store,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := r.Run(context.Background(), "user", created.Session.ID(), message.NewUserMessage("hello"), agent.RunOptions{})
	for _, err := range events {
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}

	if targetBranch != "branchful.path" {
		t.Fatalf("target branch = %q, want %q", targetBranch, "branchful.path")
	}
	if targetInvocationID == "" {
		t.Fatal("target invocation id is empty")
	}
	if targetInvocationID != sourceInvocationID {
		t.Fatalf("target invocation id = %q, want %q", targetInvocationID, sourceInvocationID)
	}

	gotSession, err := store.Get(context.Background(), &session.GetRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	entry, ok := gotSession.Session.Entries().At(gotSession.Session.Entries().Len() - 1).(*session.MessageEntry)
	if !ok {
		t.Fatalf("last persisted entry is not a MessageEntry")
	}
	if entry.Branch != "branchful.path" {
		t.Fatalf("persisted branch = %q, want %q", entry.Branch, "branchful.path")
	}
	if entry.InvocationID != sourceInvocationID {
		t.Fatalf("persisted invocation id = %q, want %q", entry.InvocationID, sourceInvocationID)
	}
}
