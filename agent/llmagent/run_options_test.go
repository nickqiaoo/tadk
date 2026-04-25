package llmagent_test

import (
	"context"
	"iter"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
	"github.com/nickqiaoo/tadk/tool/functiontool"
	"github.com/nickqiaoo/tadk/tool/interrupt"
)

func TestLLMAgentResumeDataOnlyAppliesToFirstTurnAndKeepsExtensions(t *testing.T) {
	t.Parallel()

	approvalTool, err := functiontool.New(functiontool.Config{
		Name:        "approve_once",
		Description: "requires approval",
	}, func(ctx tool.Context, _ struct{}) (map[string]any, *tool.Control, error) {
		return map[string]any{"status": "executed"}, nil, nil
	})
	if err != nil {
		t.Fatalf("functiontool.New() error = %v", err)
	}
	approvalTool = interrupt.WithApproval(approvalTool, "approve tool execution")

	probe := &turnProbeExtension{}
	root, err := llmagent.New(llmagent.Config{
		Name:  "root",
		Model: &resumeAwareModel{},
		Tools: []tool.Tool{approvalTool},
	})
	if err != nil {
		t.Fatalf("llmagent.New() error = %v", err)
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

	var interruptData *resume.InterruptData
	events := r.Run(t.Context(), "user", created.Session.ID(), message.NewUserMessage("hello"), agent.RunOptions{})
	for output, err := range events {
		if err != nil {
			t.Fatalf("first Run() error = %v", err)
		}
		if intr, ok := output.(*event.Interrupt); ok {
			interruptData = intr.Interrupt
			break
		}
	}
	if interruptData == nil {
		t.Fatal("expected interrupt event, got none")
	}

	resumeData := make(map[string]any)
	for _, ictx := range interruptData.Contexts {
		if ictx.IsRootCause {
			resumeData[ictx.ID] = map[string]any{"approved": true}
		}
	}
	if len(resumeData) == 0 {
		t.Fatal("expected resume data targets, got none")
	}

	events = r.Run(t.Context(), "user", created.Session.ID(), nil, agent.RunOptions{
		ResumeData: resumeData,
		Extensions: []agent.Extension{probe},
	})
	for _, err := range events {
		if err != nil {
			t.Fatalf("resume Run() error = %v", err)
		}
	}

	wantResumeFlags := []bool{true, false}
	if len(probe.resumeFlags) != len(wantResumeFlags) {
		t.Fatalf("turn count = %d, want %d", len(probe.resumeFlags), len(wantResumeFlags))
	}
	for i, want := range wantResumeFlags {
		if probe.resumeFlags[i] != want {
			t.Fatalf("turn %d resume flag = %v, want %v", i, probe.resumeFlags[i], want)
		}
		if probe.extensionCounts[i] != 1 {
			t.Fatalf("turn %d extension count = %d, want 1", i, probe.extensionCounts[i])
		}
	}
}

type turnProbeExtension struct {
	agent.DefaultExtension
	resumeFlags     []bool
	extensionCounts []int
}

func (e *turnProbeExtension) Name() string { return "turn-probe" }

func (e *turnProbeExtension) BeforeTurn(ctx agent.InvocationContext, ctrl *agent.TurnControl) error {
	runState := ctx.RunState()
	runConfig := ctx.RunConfig()
	e.resumeFlags = append(e.resumeFlags, len(runState.ResumeData) > 0)
	e.extensionCounts = append(e.extensionCounts, len(runConfig.Extensions))
	return nil
}

type resumeAwareModel struct{}

func (m *resumeAwareModel) Name() string { return "resume-aware-model" }

func (m *resumeAwareModel) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	for _, msg := range req.Messages {
		if msg != nil && msg.Role == message.RoleToolResult {
			return message.NewAssistantMessage("done"), nil
		}
	}
	return &message.Message{
		Role: message.RoleAssistant,
		Content: []message.Content{
			&message.ToolCallContent{
				ID:        "call-1",
				Name:      "approve_once",
				Arguments: map[string]any{},
			},
		},
		StopReason: message.StopReasonToolUse,
	}, nil
}

func (m *resumeAwareModel) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	msg, err := m.Generate(ctx, req)
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			if err != nil {
				yield(nil, err)
				return
			}
			if msg != nil {
				yield(event.NewMessageStart(msg), nil)
				yield(event.NewMessageEnd(msg), nil)
			}
		},
		func() (*message.Message, error) {
			return message.Clone(msg), err
		},
	)
}

func drain(events iter.Seq2[event.Event, error]) error {
	for _, err := range events {
		if err != nil {
			return err
		}
	}
	return nil
}
