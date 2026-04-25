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

// TestRunner_UserMessagePersistedOnlyOnce is a regression test for the P0
// double-write bug: llmagent re-emits the incoming user message as a
// MessageStart/MessageEnd pair, which used to cause persistEvent to duplicate
// the user entry already written by persistUserMessage.
func TestRunner_UserMessagePersistedOnlyOnce(t *testing.T) {
	t.Parallel()

	root := mustNewAgent(t, agent.Config{
		Name: "root",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				// Re-emit the user message the way llmagent does on firstTurn.
				if userMsg := ctx.UserContent(); userMsg != nil {
					if !yield(event.NewMessageStart(userMsg), nil) {
						return
					}
					if !yield(event.NewMessageEnd(userMsg), nil) {
						return
					}
				}
				yield(event.NewMessageEnd(
					message.NewAssistantMessage("hi"),
					event.WithAuthor("root"),
					event.WithBranch(ctx.Branch()),
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
		Agent:          root,
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

	got, err := store.Get(context.Background(), &session.GetRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	entries := got.Session.Entries()
	userCount := 0
	for i := 0; i < entries.Len(); i++ {
		me, ok := entries.At(i).(*session.MessageEntry)
		if !ok || me.Message == nil {
			continue
		}
		if me.Message.Role == message.RoleUser {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("persisted user entries = %d, want 1", userCount)
	}
}
