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

type recordingExtension struct {
	agent.DefaultExtension
	beforeRunCalls int
	onEventCalls   int
	afterRunCalls  int
}

func (e *recordingExtension) Name() string { return "recording-extension" }

func (e *recordingExtension) BeforeRun(ctx agent.InvocationContext, ctrl *agent.RunControl) (*message.Message, error) {
	e.beforeRunCalls++
	return message.NewAssistantMessage("before-run"), nil
}

func (e *recordingExtension) OnEvent(ctx agent.InvocationContext, output event.Event) (event.Event, error) {
	e.onEventCalls++
	return event.NewMessageEnd(message.NewAssistantMessage("on-event")).(*event.MessageEnd), nil
}

func (e *recordingExtension) AfterRun(ctx agent.InvocationContext, ctrl *agent.RunControl) error {
	e.afterRunCalls++
	return nil
}

func TestRunnerExtensionsShortCircuitAndTransformOutput(t *testing.T) {
	t.Parallel()

	runCalls := 0
	root := mustNewAgent(t, agent.Config{
		Name: "root",
		Run: func(ctx agent.InvocationContext) iter.Seq2[event.Event, error] {
			return func(yield func(event.Event, error) bool) {
				runCalls++
				yield(event.NewMessageEnd(
					message.NewAssistantMessage("agent-run"),
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

	ext := &recordingExtension{}
	r, err := New(Config{
		AppName:        "app",
		Agent:          root,
		SessionService: store,
		Extensions:     []agent.Extension{ext},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := r.Run(context.Background(), "user", created.Session.ID(), message.NewUserMessage("hello"), agent.RunOptions{})
	var outputs []event.Event
	for output, err := range events {
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		msgEnd, ok := output.(*event.MessageEnd)
		if ok && msgEnd.Message != nil && msgEnd.Message.Role == message.RoleAssistant {
			outputs = append(outputs, output)
		}
	}

	if runCalls != 0 {
		t.Fatalf("agent run calls = %d, want 0 due to BeforeRun short-circuit", runCalls)
	}
	if ext.beforeRunCalls != 1 || ext.onEventCalls != 1 || ext.afterRunCalls != 1 {
		t.Fatalf("extension calls = before:%d on:%d after:%d, want 1/1/1", ext.beforeRunCalls, ext.onEventCalls, ext.afterRunCalls)
	}
	if len(outputs) != 1 {
		t.Fatalf("len(outputs) = %d, want 1 assistant message_end", len(outputs))
	}
	if got := outputs[0].(*event.MessageEnd).Message.Text(); got != "on-event" {
		t.Fatalf("output text = %q, want %q", got, "on-event")
	}

	gotSession, err := store.Get(context.Background(), &session.GetRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if gotSession.Session.Entries().Len() != 2 {
		t.Fatalf("persisted entries = %d, want 2", gotSession.Session.Entries().Len())
	}
	entry, ok := gotSession.Session.Entries().At(1).(*session.MessageEntry)
	if !ok {
		t.Fatalf("persisted entry is not a MessageEntry")
	}
	if got := entry.Message.Text(); got != "on-event" {
		t.Fatalf("persisted assistant text = %q, want %q", got, "on-event")
	}
}

func mustNewAgent(t *testing.T, cfg agent.Config) agent.Agent {
	t.Helper()
	a, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	return a
}
