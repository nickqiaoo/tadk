package sequentialagent_test

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/sequentialagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
)

func TestNewSequentialAgent(t *testing.T) {
	type args struct {
		maxIterations uint
		subAgents     []agent.Agent
	}

	sameAgent := newSequentialAgent(t, []agent.Agent{newCustomAgent(t, 1), newCustomAgent(t, 2)}, "same_agent")

	tests := []struct {
		name           string
		args           args
		wantEvents     []*session.MessageEntry
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "ok",
			args: args{
				maxIterations: 0,
				subAgents:     []agent.Agent{newCustomAgent(t, 0), newCustomAgent(t, 1)},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_0",
					Message: message.NewAssistantMessage("hello 0"),
				},
				{
					Author:  "custom_agent_1",
					Message: message.NewAssistantMessage("hello 1"),
				},
			},
		},
		{
			name: "ok with inner sequential",
			args: args{
				maxIterations: 0,
				subAgents:     []agent.Agent{newCustomAgent(t, 0), newSequentialAgent(t, []agent.Agent{newCustomAgent(t, 1), newCustomAgent(t, 2)}, "test_agent1"), newCustomAgent(t, 3)},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_0",
					Message: message.NewAssistantMessage("hello 0"),
				},
				{
					Author:  "custom_agent_1",
					Message: message.NewAssistantMessage("hello 1"),
				},
				{
					Author:  "custom_agent_2",
					Message: message.NewAssistantMessage("hello 2"),
				},
				{
					Author:  "custom_agent_3",
					Message: message.NewAssistantMessage("hello 3"),
				},
			},
		},
		{
			name: "ok with same agent",
			args: args{
				maxIterations: 1,
				subAgents:     []agent.Agent{sameAgent},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_1",
					Message: message.NewAssistantMessage("hello 1"),
				},
				{
					Author:  "custom_agent_2",
					Message: message.NewAssistantMessage("hello 2"),
				},
			},
		},
		{
			name: "max iterations reached",
			args: args{
				maxIterations: 1,
				subAgents:     []agent.Agent{newCustomAgent(t, 0), newCustomAgent(t, 1), newCustomAgent(t, 2)},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_0",
					Message: message.NewAssistantMessage("hello 0"),
				},
				{
					Author:  "custom_agent_1",
					Message: message.NewAssistantMessage("hello 1"),
				},
				{
					Author:  "custom_agent_2",
					Message: message.NewAssistantMessage("hello 2"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sa, err := sequentialagent.New(sequentialagent.Config{
				AgentConfig: agent.Config{
					Name:        "test_sequential",
					Description: "test",
					SubAgents:   tt.args.subAgents,
				},
			})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}

			sessionService := session.InMemoryService()
			r, err := runner.New(runner.Config{
				AppName:        "test",
				Agent:          sa,
				SessionService: sessionService,
			})
			if err != nil {
				t.Fatalf("runner.New() error: %v", err)
			}
			if _, err := sessionService.Create(context.Background(), &session.CreateRequest{
				AppName:   "test",
				UserID:    "user",
				SessionID: "session",
			}); err != nil {
				t.Fatalf("Create() error: %v", err)
			}

			events := r.Run(context.Background(), "user", "session", message.NewUserMessage("user input"), agent.RunOptions{})
			for _, err := range events {
				if err != nil {
					if tt.wantErr {
						return
					}
					t.Fatalf("Run() error: %v", err)
				}
			}

			gotSession, err := sessionService.Get(context.Background(), &session.GetRequest{
				AppName:   "test",
				UserID:    "user",
				SessionID: "session",
			})
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			var gotEvents []*session.MessageEntry
			for entry := range gotSession.Session.Entries().All() {
				msgEntry, ok := entry.(*session.MessageEntry)
				if ok && msgEntry != nil && msgEntry.Author != "user" {
					gotEvents = append(gotEvents, msgEntry)
				}
			}

			if diff := cmp.Diff(tt.wantEvents, gotEvents,
				cmpopts.IgnoreFields(session.MessageEntry{}, "InvocationID"),
				cmpopts.IgnoreFields(session.EntryBase{}, "Type", "ID", "Timestamp", "ParentID"),
			); diff != "" {
				t.Errorf("entries mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func newCustomAgent(t *testing.T, index int) agent.Agent {
	t.Helper()
	a, err := agent.New(agent.Config{
		Name:        fmt.Sprintf("custom_agent_%d", index),
		Description: "custom agent",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				yield(event.NewMessageEnd(
					message.NewAssistantMessage(fmt.Sprintf("hello %d", index)),
					event.WithAuthor(fmt.Sprintf("custom_agent_%d", index)),
				), nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New() error: %v", err)
	}
	return a
}

func newSequentialAgent(t *testing.T, subAgents []agent.Agent, name string) agent.Agent {
	t.Helper()
	sa, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        name,
			Description: "sequential agent",
			SubAgents:   subAgents,
		},
	})
	if err != nil {
		t.Fatalf("sequentialagent.New() error: %v", err)
	}
	return sa
}

// FakeLLM is a mock implementation of model.ModelAdapter for testing.
type FakeLLM struct {
	Responses []*message.Message
	Calls     int
}

func (f *FakeLLM) Name() string { return "fake" }

func (f *FakeLLM) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	f.Calls++
	if f.Calls <= len(f.Responses) {
		return message.Clone(f.Responses[f.Calls-1]), nil
	}
	return nil, nil
}

func (f *FakeLLM) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	msg, err := f.Generate(ctx, req)
	builder := model.NewAssistantEventBuilder()
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			if err != nil {
				yield(nil, err)
				return
			}
			if msg == nil {
				return
			}
			if text := msg.Text(); text != "" {
				for _, ev := range builder.TextDelta(text) {
					if !yield(ev, nil) {
						return
					}
				}
				for _, ev := range builder.FinalizeTextThinking() {
					if !yield(ev, nil) {
						return
					}
				}
			}
			yield(builder.Done(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), err },
	)
}
