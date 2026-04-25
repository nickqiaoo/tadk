package parallelagent_test

import (
	"context"
	"fmt"
	"iter"
	rand "math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/loopagent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/parallelagent"
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

func TestNewParallelAgent(t *testing.T) {
	tests := []struct {
		name          string
		maxIterations uint
		numSubAgents  int
		agentError    error
		cancelContext bool
		wantEvents    []*session.MessageEntry
		wantErr       bool
	}{
		{
			name:          "subagents complete run",
			maxIterations: 2,
			numSubAgents:  3,
			wantEvents: func() []*session.MessageEntry {
				var res []*session.MessageEntry
				for agentID := 1; agentID <= 3; agentID++ {
					for responseCount := 1; responseCount <= 2; responseCount++ {
						res = append(res, &session.MessageEntry{
							Author:  fmt.Sprintf("sub%d", agentID),
							Message: message.NewAssistantMessage(fmt.Sprintf("hello %d", agentID)),
						})
					}
				}
				return res
			}(),
		},
		{
			name:          "handle ctx cancel",
			maxIterations: 0,
			cancelContext: true,
			wantErr:       true,
		},
		{
			name:          "agent returns error",
			maxIterations: 0,
			numSubAgents:  100,
			agentError:    fmt.Errorf("agent error"),
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			parallelAgent := newParallelAgent(t, tt.maxIterations, tt.numSubAgents, tt.agentError)

			var gotEvents []*session.MessageEntry

			sessionService := session.InMemoryService()

			agentRunner, err := runner.New(runner.Config{
				AppName:        "test_app",
				Agent:          parallelAgent,
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

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			if tt.cancelContext {
				go func() {
					time.Sleep(5 * time.Millisecond)
					cancel()
				}()
			}

			events := agentRunner.Run(ctx, "user_id", "session_id", message.NewUserMessage("user input"), agent.RunOptions{})
			for output, err := range events {
				if tt.wantErr != (err != nil) {
					if tt.cancelContext && err == nil {
						continue
					}
					if tt.agentError != nil && err == nil {
						continue
					}
					t.Errorf("got unexpected error: %v", err)
				}

				if output == nil {
					continue
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
			for entry := range gotSession.Session.Entries().All() {
				msgEntry, ok := entry.(*session.MessageEntry)
				if ok && msgEntry != nil && msgEntry.Author != "user" {
					gotEvents = append(gotEvents, msgEntry)
				}
			}

			if tt.wantEvents != nil {
				eventCompareFunc := func(e1, e2 *session.MessageEntry) int {
					if e1.Author <= e2.Author {
						return -1
					}
					if e1.Author == e2.Author {
						return 0
					}
					return 1
				}

				slices.SortFunc(tt.wantEvents, eventCompareFunc)
				slices.SortFunc(gotEvents, eventCompareFunc)

				if diff := cmp.Diff(tt.wantEvents, gotEvents,
					cmpopts.IgnoreFields(session.MessageEntry{}, "ID", "Timestamp", "InvocationID", "Branch"),
					cmpopts.IgnoreFields(session.EntryBase{}, "Type", "ParentID"),
				); diff != "" {
					t.Errorf("entries mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func newParallelAgent(t *testing.T, maxIterations uint, numSubAgents int, agentErr error) agent.Agent {
	var subAgents []agent.Agent

	for i := 1; i <= numSubAgents; i++ {
		subAgents = append(subAgents, must(loopagent.New(loopagent.Config{
			MaxIterations: maxIterations,
			AgentConfig: agent.Config{
				Name: fmt.Sprintf("loop_agent_%d", i),
				SubAgents: []agent.Agent{
					must(agent.New(agent.Config{
						Name: fmt.Sprintf("sub%d", i),
						Run:  customRun(i, nil),
					},
					)),
				},
			},
		})))
	}

	if agentErr != nil {
		subAgents = append(subAgents, must(agent.New(agent.Config{
			Name: "error_agent",
			Run:  customRun(-1, agentErr),
		})))
	}

	agent, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      "test_agent",
			SubAgents: subAgents,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return agent
}

func must[T agent.Agent](a T, err error) T {
	if err != nil {
		panic(err)
	}
	return a
}

func customRun(id int, agentErr error) func(agent.InvocationContext) iter.Seq2[event.Event, error] {
	return func(agent.InvocationContext) iter.Seq2[event.Event, error] {
		return func(yield func(event.Event, error) bool) {
			time.Sleep((time.Duration(rand.IntN(5) + 1)) * time.Millisecond)
			if agentErr != nil {
				yield(nil, agentErr)
				return
			}
			yield(event.NewMessageEnd(
				message.NewAssistantMessage(fmt.Sprintf("hello %v", id)),
				event.WithAuthor(fmt.Sprintf("sub%v", id)),
			), nil)
		}
	}
}

// TestParallelAgentResume tests the resume functionality of parallel agent with multiple sub-agents.
func TestParallelAgentResume(t *testing.T) {
	ctx := t.Context()

	approvalTool1 := createParallelApprovalTool(t, "tool_1")
	approvalTool2 := createParallelApprovalTool(t, "tool_2")

	subAgent1 := newParallelLLMAgent(t, "sub_agent_1", approvalTool1)
	subAgent2 := newParallelLLMAgent(t, "sub_agent_2", approvalTool2)

	pAgent, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      "parallel_agent",
			SubAgents: []agent.Agent{subAgent1, subAgent2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionService := session.InMemoryService()
	agentRunner, err := runner.New(runner.Config{
		AppName:        "test_app",
		Agent:          pAgent,
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

	var interruptData *resume.InterruptData
	events := agentRunner.Run(ctx, "user_id", "session_id", message.NewUserMessage("do both tasks"), agent.RunOptions{})
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
	parallelState, err := agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "parallel_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after interrupt: %v", err)
	}
	if parallelState == nil {
		t.Fatal("expected parallel agent state after interrupt")
	}

	t.Logf("Interrupt Address2ID: %v", interruptData.Address2ID)
	t.Logf("Interrupt Contexts count: %d", len(interruptData.Contexts))

	resumeData := make(map[string]any)
	for _, ictx := range interruptData.Contexts {
		if ictx.IsRootCause {
			t.Logf("Approving interrupt: id=%s, addr=%s", ictx.ID, ictx.Address)
			resumeData[ictx.ID] = map[string]any{"approved": true}
		}
	}

	if len(resumeData) == 0 {
		t.Fatal("no interrupts to approve")
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
	resumeInterrupted := false
	for output, err := range resumeEventsStream {
		if err != nil {
			t.Fatalf("unexpected error during resume: %v", err)
		}
		if output != nil && output.EventType() == event.TypeInterrupt {
			t.Log("Got another interrupt after resume")
			resumeInterrupted = true
			break
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

	if len(resumeEvents) == 0 {
		t.Fatal("expected entries after resume, got none")
	}

	tool1Executed := false
	tool2Executed := false
	for _, ev := range resumeEvents {
		if ev.Message != nil {
			for _, tr := range ev.Message.ToolResults() {
				if m, ok := tr.Content.(map[string]any); ok {
					if name, ok := m["tool"].(string); ok {
						if name == "tool_1" {
							tool1Executed = true
						}
						if name == "tool_2" {
							tool2Executed = true
						}
					}
				}
			}
		}
	}

	if !tool1Executed {
		t.Error("tool_1 was not executed after resume")
	}
	if !tool2Executed {
		t.Error("tool_2 was not executed after resume")
	}

	parallelState, err = agentStateSvc.GetAgentState(ctx, &session.AgentStateRequest{
		AppName:      "test_app",
		UserID:       "user_id",
		SessionID:    "session_id",
		AgentAddress: "parallel_agent",
	})
	if err != nil {
		t.Fatalf("GetAgentState() after resume: %v", err)
	}
	if resumeInterrupted {
		if parallelState == nil {
			t.Fatal("expected parallel agent state to remain when resume interrupts again")
		}
	} else if parallelState != nil {
		t.Fatal("expected parallel agent state to be cleared after resume completion")
	}
}

type EmptyArgs struct{}

func createParallelApprovalTool(t *testing.T, name string) tool.Tool {
	t.Helper()

	baseTool, err := functiontool.New(functiontool.Config{
		Name:        name,
		Description: fmt.Sprintf("A tool named %s that requires approval", name),
	}, func(ctx tool.Context, args EmptyArgs) (map[string]any, *tool.Control, error) {
		t.Logf("Tool %s executed!", name)
		return map[string]any{"status": "executed", "tool": name}, nil, nil
	})
	if err != nil {
		t.Fatalf("error creating tool %s: %v", name, err)
	}

	return interrupt.WithApproval(baseTool, fmt.Sprintf("Please approve %s", name))
}

func newParallelLLMAgent(t *testing.T, name string, approvalTool tool.Tool) agent.Agent {
	t.Helper()

	a, err := llmagent.New(llmagent.Config{
		Name:        name,
		Model:       &ParallelApprovalLLM{toolName: approvalTool.Name()},
		Description: fmt.Sprintf("Agent %s with approval tool", name),
		Tools:       []tool.Tool{approvalTool},
	})
	if err != nil {
		t.Fatal(err)
	}

	return a
}

type ParallelApprovalLLM struct {
	toolName    string
	callCounter int
}

func (f *ParallelApprovalLLM) Name() string {
	return "parallel-approval-llm"
}

func (f *ParallelApprovalLLM) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
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
		return message.NewAssistantMessage("Task completed successfully"), nil
	}

	return &message.Message{
		Role: message.RoleAssistant,
		Content: []message.Content{
			&message.ToolCallContent{
				ID:        fmt.Sprintf("call-%d", f.callCounter),
				Name:      f.toolName,
				Arguments: map[string]any{},
			},
		},
	}, nil
}

func (f *ParallelApprovalLLM) Stream(ctx context.Context, req *model.Request) *model.EventStream {
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
