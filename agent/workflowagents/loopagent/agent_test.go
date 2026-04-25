package loopagent_test

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/loopagent"
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

func TestNewLoopAgent(t *testing.T) {
	type args struct {
		maxIterations uint
		subAgents     []agent.Agent
	}

	tests := []struct {
		name       string
		args       args
		wantEvents []*session.MessageEntry
		wantErr    bool
	}{
		{
			name: "infinite loop",
			args: args{
				maxIterations: 0,
				subAgents:     []agent.Agent{newCustomAgent(t, 0)},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_0",
					Message: message.NewAssistantMessage("hello 0"),
				},
			},
		},
		{
			name: "loop agent with max iterations",
			args: args{
				maxIterations: 1,
				subAgents:     []agent.Agent{newCustomAgent(t, 0)},
			},
			wantEvents: []*session.MessageEntry{
				{
					Author:  "custom_agent_0",
					Message: message.NewAssistantMessage("hello 0"),
				},
			},
		},
		{
			name: "loop agent with max iterations and 2 sub agents",
			args: args{
				maxIterations: 1,
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			loopAgent, err := loopagent.New(loopagent.Config{
				MaxIterations: tt.args.maxIterations,
				AgentConfig: agent.Config{
					Name:      "test_agent",
					SubAgents: tt.args.subAgents,
				},
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLoopAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			sessionService := session.InMemoryService()

			agentRunner, err := runner.New(runner.Config{
				AppName:        "test_app",
				Agent:          loopAgent,
				SessionService: sessionService,
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = sessionService.Create(ctx, &session.CreateRequest{
				AppName:   "test_app",
				UserID:    "user_id",
				SessionID: "session_id",
			})
			if err != nil {
				t.Fatal(err)
			}

			events := agentRunner.Run(ctx, "user_id", "session_id", message.NewUserMessage("user input"), agent.RunOptions{})
			for output, err := range events {
				if err != nil {
					t.Errorf("got unexpected error: %v", err)
				}
				if tt.args.maxIterations == 0 && len(tt.wantEvents) > 0 && output != nil && output.EventType() == event.TypeMessageEnd {
					gotSession, err := sessionService.Get(ctx, &session.GetRequest{
						AppName:   "test_app",
						UserID:    "user_id",
						SessionID: "session_id",
					})
					if err == nil && gotSession.Session.Entries().Len() >= len(tt.wantEvents) {
						break
					}
				}
			}

			gotSession, err := sessionService.Get(ctx, &session.GetRequest{
				AppName:   "test_app",
				UserID:    "user_id",
				SessionID: "session_id",
			})
			if err != nil {
				t.Fatal(err)
			}
			var gotEvents []*session.MessageEntry
			for entry := range gotSession.Session.Entries().All() {
				msgEntry, ok := entry.(*session.MessageEntry)
				if ok && msgEntry != nil && msgEntry.Author != "user" {
					gotEvents = append(gotEvents, msgEntry)
				}
			}

			if tt.args.maxIterations == 0 && len(gotEvents) > len(tt.wantEvents) {
				gotEvents = gotEvents[:len(tt.wantEvents)]
			}

			if len(tt.wantEvents) != len(gotEvents) {
				t.Fatalf("Unexpected event length, got: %v, want: %v", len(gotEvents), len(tt.wantEvents))
			}

			ignoreFields := []cmp.Option{
				cmpopts.IgnoreFields(session.MessageEntry{}, "ID", "InvocationID", "Timestamp"),
				cmpopts.IgnoreFields(session.EntryBase{}, "ParentID", "Type"),
			}

			for i, gotEvent := range gotEvents {
				tt.wantEvents[i].Timestamp = gotEvent.Timestamp
				if diff := cmp.Diff(tt.wantEvents[i], gotEvent, ignoreFields...); diff != "" {
					t.Errorf("event[%v] mismatch (-want +got):\n%s", i, diff)
				}
			}
		})
	}
}

func newCustomAgent(t *testing.T, id int) agent.Agent {
	t.Helper()

	customAgent := &customAgent{
		id: id,
	}

	a, err := agent.New(agent.Config{
		Name: fmt.Sprintf("custom_agent_%v", id),
		Run:  customAgent.Run,
	})
	if err != nil {
		t.Fatal(err)
	}

	return a
}

type customAgent struct {
	id          int
	callCounter int
}

func (a *customAgent) Run(agent.InvocationContext) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		a.callCounter++

		yield(event.NewMessageEnd(
			message.NewAssistantMessage(fmt.Sprintf("hello %v", a.id)),
			event.WithAuthor(fmt.Sprintf("custom_agent_%v", a.id)),
		), nil)
	}
}

type EmptyArgs struct{}

func exampleFunctionThatEscalates(ctx tool.Context, myArgs EmptyArgs) (map[string]string, *tool.Control, error) {
	return map[string]string{}, &tool.Control{ExitToParent: true}, nil
}

func exampleFunctionThatEscalatesAndSkips(ctx tool.Context, myArgs EmptyArgs) (map[string]string, *tool.Control, error) {
	return map[string]string{}, &tool.Control{ExitToParent: true, Terminal: true}, nil
}

func newLLMAgentWithFunctionCall(t *testing.T, id int, skipSummarization bool) agent.Agent {
	t.Helper()

	exampleFunction := exampleFunctionThatEscalates
	if skipSummarization {
		exampleFunction = exampleFunctionThatEscalatesAndSkips
	}

	exampleFunctionThatEscalatesTool, err := functiontool.New(functiontool.Config{
		Name:        "exampleFunction",
		Description: "Call this function to escalate\n",
	}, exampleFunction)
	if err != nil {
		t.Fatalf("error creating exampleFunction tool: %s", err)
	}

	customAgent, err := llmagent.New(llmagent.Config{
		Name:  fmt.Sprintf("custom_agent_%v", id),
		Model: &FakeLLM{id: id, callCounter: 0, skipSummarization: skipSummarization},
		Tools: []tool.Tool{exampleFunctionThatEscalatesTool},
	})
	if err != nil {
		t.Fatal(err)
	}

	return customAgent
}

// FakeLLM is a mock implementation of model.ModelAdapter for testing.
type FakeLLM struct {
	id                int
	callCounter       int
	skipSummarization bool
}

func (f *FakeLLM) Name() string {
	return "fake-llm"
}

func (f *FakeLLM) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	f.callCounter++
	if len(req.Messages) == 1 {
		return &message.Message{
			Role: message.RoleAssistant,
			Content: []message.Content{
				&message.ToolCallContent{
					Name:      "exampleFunction",
					Arguments: make(map[string]any),
				},
			},
		}, nil
	}
	return message.NewAssistantMessage(fmt.Sprintf("hello %v", f.id)), nil
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
			for _, tc := range msg.ToolCalls() {
				for _, ev := range builder.ToolCall(tc.ID, tc.Name, tc.Args) {
					if !yield(ev, nil) {
						return
					}
				}
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
			for _, ev := range builder.FinalizeToolCalls() {
				if !yield(ev, nil) {
					return
				}
			}
			yield(builder.Done(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), err },
	)
}

// TestLoopAgentResume tests the resume functionality of the loop agent.
func TestLoopAgentResume(t *testing.T) {
	ctx := t.Context()

	// Create a tool that requires approval (will interrupt)
	approvalTool := createApprovalTool(t)

	// Create sub-agents: first one has approval tool, second is simple
	subAgent0 := newLLMAgentWithApprovalTool(t, 0, approvalTool)
	subAgent1 := newCustomAgent(t, 1)

	loopAgent, err := loopagent.New(loopagent.Config{
		MaxIterations: 2,
		AgentConfig: agent.Config{
			Name:      "loop_agent",
			SubAgents: []agent.Agent{subAgent0, subAgent1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionService := session.InMemoryService()
	agentRunner, err := runner.New(runner.Config{
		AppName:        "test_app",
		Agent:          loopAgent,
		SessionService: sessionService,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}

	// First run - should interrupt at the approval tool
	var interruptData *resume.InterruptData
	events := agentRunner.Run(ctx, "user_id", "session_id", message.NewUserMessage("user input"), agent.RunOptions{})
	for output, err := range events {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != nil && output.EventType() == event.TypeInterrupt {
			interruptData = output.(*event.Interrupt).Interrupt
			break
		}
	}

	if interruptData == nil {
		t.Fatal("expected interrupt event, got none")
	}

	agentStateSvc, ok := sessionService.(session.Service)
	if !ok {
		t.Fatal("session service does not implement AgentStateService")
	}
	loopState, err := agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "loop_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after interrupt: %v", err)
	}
	if loopState == nil {
		t.Fatal("expected loop agent state after interrupt")
	}

	// Verify interrupt data contains loop state
	t.Logf("Interrupt Address2ID: %v", interruptData.Address2ID)
	t.Logf("Interrupt ID2State: %v", interruptData.ID2State)
	t.Logf("Interrupt Contexts: %+v", interruptData.Contexts)

	// Find the interrupt ID for the approval tool
	var approvalInterruptID string
	for _, ictx := range interruptData.Contexts {
		if ictx.IsRootCause {
			approvalInterruptID = ictx.ID
			break
		}
	}

	if approvalInterruptID == "" {
		t.Fatal("could not find approval interrupt ID")
	}

	// Resume with approval
	resumeData := map[string]any{
		approvalInterruptID: map[string]any{"approved": true},
	}

	beforeResume, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	startIdx := beforeResume.Session.Entries().Len()
	resumeEventsStream := agentRunner.Run(ctx, "user_id", "session_id", nil, agent.RunOptions{ResumeData: resumeData})
	for _, err := range resumeEventsStream {
		if err != nil {
			t.Fatalf("unexpected error during resume: %v", err)
		}
	}
	afterResume, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	var resumeEvents []*session.MessageEntry
	for i := startIdx; i < afterResume.Session.Entries().Len(); i++ {
		if msgEntry, ok := afterResume.Session.Entries().At(i).(*session.MessageEntry); ok {
			resumeEvents = append(resumeEvents, msgEntry)
		}
	}

	// Verify we got entries after resume
	if len(resumeEvents) == 0 {
		t.Fatal("expected entries after resume, got none")
	}

	// Check that we got the function response (tool executed after approval)
	foundFunctionResponse := false
	for _, ev := range resumeEvents {
		if ev.Message != nil {
			for _, tr := range ev.Message.ToolResults() {
				if tr.CallID != "" {
					foundFunctionResponse = true
					if m, ok := tr.Content.(map[string]any); ok && m["status"] == "executed" {
						t.Log("Tool was executed successfully after resume")
					}
				}
			}
		}
	}

	if !foundFunctionResponse {
		t.Error("expected function response after resume")
	}

	loopState, err = agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "loop_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after resume: %v", err)
	}
	if loopState != nil {
		t.Fatal("expected loop agent state to be cleared after resume")
	}
}

// TestLoopAgentResumeSecondSubAgent tests resume when interrupt happens in the second sub-agent.
func TestLoopAgentResumeSecondSubAgent(t *testing.T) {
	ctx := t.Context()

	// Create a tool that requires approval (will interrupt)
	approvalTool := createApprovalTool(t)

	// Create sub-agents: first one is simple, second has approval tool
	subAgent0 := newCustomAgent(t, 0)
	subAgent1 := newLLMAgentWithApprovalTool(t, 1, approvalTool)

	loopAgent, err := loopagent.New(loopagent.Config{
		MaxIterations: 2,
		AgentConfig: agent.Config{
			Name:      "loop_agent",
			SubAgents: []agent.Agent{subAgent0, subAgent1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionService := session.InMemoryService()
	agentRunner, err := runner.New(runner.Config{
		AppName:        "test_app",
		Agent:          loopAgent,
		SessionService: sessionService,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}

	// First run - should get event from subAgent0, then interrupt at subAgent1
	var interruptData2 *resume.InterruptData
	events2 := agentRunner.Run(ctx, "user_id", "session_id", message.NewUserMessage("user input"), agent.RunOptions{})
	for output, err := range events2 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output != nil && output.EventType() == event.TypeInterrupt {
			interruptData2 = output.(*event.Interrupt).Interrupt
			break
		}
	}
	gotSession, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	var entries []*session.MessageEntry
	for entry := range gotSession.Session.Entries().All() {
		if msgEntry, ok := entry.(*session.MessageEntry); ok {
			entries = append(entries, msgEntry)
		}
	}

	// Verify we got event from first sub-agent before interrupt
	foundFirstAgent := false
	for _, ev := range entries {
		if ev.Author == "custom_agent_0" {
			foundFirstAgent = true
		}
	}
	if !foundFirstAgent {
		t.Error("expected event from first sub-agent before interrupt")
	}

	if interruptData2 == nil {
		t.Fatal("expected interrupt event, got none")
	}

	agentStateSvc, ok := sessionService.(session.Service)
	if !ok {
		t.Fatal("session service does not implement AgentStateService")
	}
	loopState, err := agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "loop_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after interrupt: %v", err)
	}
	if loopState == nil {
		t.Fatal("expected loop agent state after interrupt")
	}

	// Find the interrupt ID for the approval tool
	var approvalInterruptID string
	for _, ictx := range interruptData2.Contexts {
		if ictx.IsRootCause {
			approvalInterruptID = ictx.ID
			break
		}
	}

	if approvalInterruptID == "" {
		t.Fatal("could not find approval interrupt ID")
	}

	// Resume with approval
	resumeData := map[string]any{
		approvalInterruptID: map[string]any{"approved": true},
	}

	beforeResume2, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	startIdx2 := beforeResume2.Session.Entries().Len()
	resumeEventsStream2 := agentRunner.Run(ctx, "user_id", "session_id", nil, agent.RunOptions{ResumeData: resumeData})
	for _, err := range resumeEventsStream2 {
		if err != nil {
			t.Fatalf("unexpected error during resume: %v", err)
		}
	}
	afterResume2, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   "test_app",
		UserID:    "user_id",
		SessionID: "session_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	var resumeEvents []*session.MessageEntry
	for i := startIdx2; i < afterResume2.Session.Entries().Len(); i++ {
		if msgEntry, ok := afterResume2.Session.Entries().At(i).(*session.MessageEntry); ok {
			resumeEvents = append(resumeEvents, msgEntry)
		}
	}

	// Verify we did NOT get duplicate entries from first sub-agent
	firstAgentCountAfterResume := 0
	for _, ev := range resumeEvents {
		if ev.Author == "custom_agent_0" {
			firstAgentCountAfterResume++
		}
	}

	// After resume, we should continue with iteration 1, which means:
	// - First iteration (0): already done before interrupt
	// - After resume: continues from subAgent1, then starts iteration 1
	// In iteration 1, both sub-agents run, so we should see custom_agent_0 once
	if firstAgentCountAfterResume > 1 {
		t.Errorf("expected at most 1 event from first sub-agent after resume (for iteration 1), got %d", firstAgentCountAfterResume)
	}

	loopState, err = agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "loop_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after resume: %v", err)
	}
	if loopState != nil {
		t.Fatal("expected loop agent state to be cleared after resume")
	}
}

func createApprovalTool(t *testing.T) tool.Tool {
	t.Helper()

	baseTool, err := functiontool.New(functiontool.Config{
		Name:        "approvalFunction",
		Description: "A function that requires approval",
	}, func(ctx tool.Context, args EmptyArgs) (map[string]any, *tool.Control, error) {
		t.Log("baseTool.Run called - this should NOT happen on first call")
		return map[string]any{"status": "executed"}, nil, nil
	})
	if err != nil {
		t.Fatalf("error creating approval tool: %s", err)
	}

	return interrupt.WithApproval(baseTool, "Please approve this action")
}

func newLLMAgentWithApprovalTool(t *testing.T, id int, approvalTool tool.Tool) agent.Agent {
	t.Helper()

	customAgent, err := llmagent.New(llmagent.Config{
		Name:  fmt.Sprintf("custom_agent_%v", id),
		Model: &ApprovalLLM{id: id},
		Tools: []tool.Tool{approvalTool},
	})
	if err != nil {
		t.Fatal(err)
	}

	return customAgent
}

// ApprovalLLM is a mock LLM that always calls the approval function.
type ApprovalLLM struct {
	id          int
	callCounter int
}

func (f *ApprovalLLM) Name() string {
	return "approval-llm"
}

func (f *ApprovalLLM) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	f.callCounter++

	hasFunctionResponse := false
	for _, msg := range req.Messages {
		if msg == nil {
			continue
		}
		if len(msg.ToolResults()) > 0 {
			hasFunctionResponse = true
			break
		}
	}

	if hasFunctionResponse {
		return message.NewAssistantMessage(fmt.Sprintf("completed %v", f.id)), nil
	}
	return &message.Message{
		Role: message.RoleAssistant,
		Content: []message.Content{
			&message.ToolCallContent{
				Name:      "approvalFunction",
				Arguments: make(map[string]any),
			},
		},
	}, nil
}

func (f *ApprovalLLM) Stream(ctx context.Context, req *model.Request) *model.EventStream {
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
			for _, tc := range msg.ToolCalls() {
				for _, ev := range builder.ToolCall(tc.ID, tc.Name, tc.Args) {
					if !yield(ev, nil) {
						return
					}
				}
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
			for _, ev := range builder.FinalizeToolCalls() {
				if !yield(ev, nil) {
					return
				}
			}
			yield(builder.Done(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), err },
	)
}
