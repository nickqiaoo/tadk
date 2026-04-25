package parallelagent_test

import (
	"iter"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/parallelagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
)

func TestParallelAgentPropagatesTreeAndBuildsNestedBranches(t *testing.T) {
	t.Parallel()

	type snapshot struct {
		branch         string
		address        string
		parentName     string
		extensionCount int
	}

	var got snapshot
	leaf, err := agent.New(agent.Config{
		Name: "leaf",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				parent := ctx.Tree().ParentOf(ctx.AgentName())
				if parent != nil {
					got.parentName = parent.Name()
				}
				got.branch = ctx.Branch()
				got.address = ctx.Address()
				got.extensionCount = len(ctx.RunConfig().Extensions)
				yield(event.NewMessageEnd(
					message.NewAssistantMessage("done"),
					event.WithAuthor("leaf"),
				), nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}

	inner, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      "inner",
			SubAgents: []agent.Agent{leaf},
		},
	})
	if err != nil {
		t.Fatalf("parallelagent.New(inner) error = %v", err)
	}

	outer, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      "outer",
			SubAgents: []agent.Agent{inner},
		},
	})
	if err != nil {
		t.Fatalf("parallelagent.New(outer) error = %v", err)
	}

	store := session.InMemoryService()
	created, err := store.Create(t.Context(), &session.CreateRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "app",
		Agent:          outer,
		SessionService: store,
	})
	if err != nil {
		t.Fatalf("runner.New() error = %v", err)
	}

	events := r.Run(t.Context(), "user", created.Session.ID(), message.NewUserMessage("hello"), agent.RunOptions{
		Extensions: []agent.Extension{agent.DefaultExtension{}},
	})
	for _, err := range events {
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}

	if got.branch != "outer.inner.leaf" {
		t.Fatalf("leaf branch = %q, want %q", got.branch, "outer.inner.leaf")
	}
	if got.address != "outer.inner.leaf" {
		t.Fatalf("leaf address = %q, want %q", got.address, "outer.inner.leaf")
	}
	if got.parentName != "inner" {
		t.Fatalf("leaf parent = %q, want %q", got.parentName, "inner")
	}
	if got.extensionCount != 1 {
		t.Fatalf("leaf extension count = %d, want 1", got.extensionCount)
	}
}
