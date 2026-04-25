package loopagent_test

import (
	"iter"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/loopagent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/parallelagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
)

func TestLoopAgentPropagatesChildInvocationContext(t *testing.T) {
	t.Parallel()

	type snapshot struct {
		branch         string
		address        string
		invocationID   string
		extensionCount int
		hasTree        bool
	}

	var got snapshot
	leaf, err := agent.New(agent.Config{
		Name: "leaf",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				got = snapshot{
					branch:         ctx.Branch(),
					address:        ctx.Address(),
					invocationID:   ctx.InvocationID(),
					extensionCount: len(ctx.RunConfig().Extensions),
					hasTree:        ctx.Tree() != nil,
				}
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

	loop, err := loopagent.New(loopagent.Config{
		MaxIterations: 1,
		AgentConfig: agent.Config{
			Name:      "loop_agent",
			SubAgents: []agent.Agent{leaf},
		},
	})
	if err != nil {
		t.Fatalf("loopagent.New() error = %v", err)
	}

	root, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      "parallel_root",
			SubAgents: []agent.Agent{loop},
		},
	})
	if err != nil {
		t.Fatalf("parallelagent.New() error = %v", err)
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
		Agent:          root,
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

	if got.branch != "parallel_root.loop_agent" {
		t.Fatalf("child branch = %q, want %q", got.branch, "parallel_root.loop_agent")
	}
	if got.address != "parallel_root.loop_agent.leaf" {
		t.Fatalf("child address = %q, want %q", got.address, "parallel_root.loop_agent.leaf")
	}
	if got.invocationID == "" {
		t.Fatal("child invocation id is empty")
	}
	if got.extensionCount != 1 {
		t.Fatalf("child extension count = %d, want 1", got.extensionCount)
	}
	if !got.hasTree {
		t.Fatal("child tree = nil, want non-nil")
	}
}
