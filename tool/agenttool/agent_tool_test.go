package agenttool

import (
	"context"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

type fakeStreamingModel struct {
	generateCalls int
	streamCalls   int
}

func (m *fakeStreamingModel) Name() string { return "fake" }

func (m *fakeStreamingModel) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	m.generateCalls++
	return message.NewAssistantMessage("from generate"), nil
}

func (m *fakeStreamingModel) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	m.streamCalls++
	msg := message.NewAssistantMessage("from stream")
	builder := model.NewAssistantEventBuilder()
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			for _, ev := range builder.TextDelta(msg.Text()) {
				if !yield(ev, nil) {
					return
				}
			}
			for _, ev := range builder.FinalizeTextThinking() {
				if !yield(ev, nil) {
					return
				}
			}
			yield(builder.Done(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), nil },
	)
}

func TestAgentToolRequiresSessionService(t *testing.T) {
	subAgent, err := agent.New(agent.Config{Name: "child"})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	toolUnderTest := New(subAgent, nil)

	_, _, err = toolUnderTest.Execute(newToolContext(t, nil), map[string]any{"request": "hi"})
	if err == nil || err.Error() != "agenttool requires Config.SessionService" {
		t.Fatalf("Execute() error = %v, want agenttool requires Config.SessionService", err)
	}
}

func TestAgentToolInheritsStreamingMode(t *testing.T) {
	t.Run("non-streaming uses Generate", func(t *testing.T) {
		modelAdapter := &fakeStreamingModel{}
		toolUnderTest := newLLMAgentTool(t, modelAdapter)

		got, _, err := toolUnderTest.Execute(newToolContext(t, &agent.RunOptions{
			StreamingMode: agent.StreamingModeNone,
		}), map[string]any{"request": "hi"})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := map[string]any{"result": "from generate"}
		if !equalResult(got, want) {
			t.Fatalf("Execute() result = %#v, want %#v", got, want)
		}
		if modelAdapter.generateCalls != 1 || modelAdapter.streamCalls != 0 {
			t.Fatalf("Generate/Stream calls = %d/%d, want 1/0", modelAdapter.generateCalls, modelAdapter.streamCalls)
		}
	})

	t.Run("sse uses Stream", func(t *testing.T) {
		modelAdapter := &fakeStreamingModel{}
		toolUnderTest := newLLMAgentTool(t, modelAdapter)

		got, _, err := toolUnderTest.Execute(newToolContext(t, &agent.RunOptions{
			StreamingMode: agent.StreamingModeSSE,
		}), map[string]any{"request": "hi"})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := map[string]any{"result": "from stream"}
		if !equalResult(got, want) {
			t.Fatalf("Execute() result = %#v, want %#v", got, want)
		}
		if modelAdapter.generateCalls != 0 || modelAdapter.streamCalls != 1 {
			t.Fatalf("Generate/Stream calls = %d/%d, want 0/1", modelAdapter.generateCalls, modelAdapter.streamCalls)
		}
	})
}

func newLLMAgentTool(t *testing.T, modelAdapter model.ModelAdapter) tool.Tool {
	t.Helper()

	subAgent, err := llmagent.New(llmagent.Config{
		Name:  "child",
		Model: modelAdapter,
	})
	if err != nil {
		t.Fatalf("llmagent.New() error = %v", err)
	}

	return New(subAgent, &Config{
		SessionService: session.InMemoryService(),
	})
}

type testToolContext struct {
	base tool.Context
}

func (t *testToolContext) Context() context.Context                   { return t.base.Context() }
func (t *testToolContext) InvocationContext() agent.InvocationContext { return t.base.InvocationContext() }
func (t *testToolContext) FunctionCallID() string                     { return t.base.FunctionCallID() }

func (t *testToolContext) Interrupt(hint string, payload any) error { return t.base.Interrupt(hint, payload) }
func (t *testToolContext) ResumeData() (bool, bool, any)            { return t.base.ResumeData() }
func (t *testToolContext) InterruptState() (bool, any)              { return t.base.InterruptState() }

func newToolContext(t *testing.T, runOpts *agent.RunOptions) tool.Context {
	t.Helper()

	svc := session.InMemoryService()
	resp, err := svc.Create(context.Background(), &session.CreateRequest{
		AppName: "app",
		UserID:  "user",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	rootAgent, err := agent.New(agent.Config{Name: "root"})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}

	var runCfg *agent.RunConfig
	var runState *agent.RunState
	if runOpts != nil {
		runCfg, runState = runOpts.Split()
	}

	inv := agent.NewInvocationContext(context.Background(), agent.InvocationContextParams{
		Session:     resp.Session,
		AgentName:   rootAgent.Name(),
		Address:     rootAgent.Name(),
		Branch:      rootAgent.Name(),
		RunConfig:   runCfg,
		RunState:    runState,
		UserContent: message.NewUserMessage("hi"),
	})

	baseCtx := toolinternal.NewToolContext(inv, "tool-call-1")
	return &testToolContext{base: baseCtx}
}

func equalResult(got any, want map[string]any) bool {
	gotMap, ok := got.(map[string]any)
	if !ok {
		return false
	}
	if len(gotMap) != len(want) {
		return false
	}
	for k, wantVal := range want {
		if gotMap[k] != wantVal {
			return false
		}
	}
	return true
}
