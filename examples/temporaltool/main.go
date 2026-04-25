// Package main demonstrates wrapping a regular ADK tool with
// tool/temporaltool so only the tool call runs through Temporal.
//
// Flow:
//
//	LLMAgent -> durable Temporal tool -> interrupt for approval -> resume -> real API call
//
// The root agent remains a normal in-process ADK llmagent, so conversational
// streaming stays in ADK. Temporal is only used as a durable execution backend
// for the tool itself.
//
// Run:
//
//	Ensure a local Temporal server is listening on localhost:7233
//	go run ./examples/temporaltool
//
// Optional environment variables:
//
//	TEMPORAL_HOSTPORT   default: localhost:7233
//	TEMPORAL_TASK_QUEUE default: adk-temporal-tools-example
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.temporal.io/sdk/client"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/temporal/toolworker"
	adktool "github.com/nickqiaoo/tadk/tool"
	"github.com/nickqiaoo/tadk/tool/functiontool"
	"github.com/nickqiaoo/tadk/tool/interrupt"
	"github.com/nickqiaoo/tadk/tool/temporaltool"
)

type deployArgs struct {
	Service string `json:"service" jsonschema:"Service name to deploy."`
	Version string `json:"version" jsonschema:"Version to deploy."`
}

type deployResult struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Service     string `json:"service"`
	Version     string `json:"version"`
}

type scriptedModel struct {
	responses []*message.Message
	index     int
}

func (m *scriptedModel) Name() string {
	return "scripted"
}

func (m *scriptedModel) Generate(context.Context, *model.Request) (*message.Message, error) {
	if m.index >= len(m.responses) {
		return message.NewAssistantMessage("done"), nil
	}
	msg := message.Clone(m.responses[m.index])
	m.index++
	return msg, nil
}

func (m *scriptedModel) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	msg, err := m.Generate(ctx, req)
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			if err != nil {
				yield(nil, err)
				return
			}
			yield(event.NewMessageEnd(msg), nil)
		},
		func() (*message.Message, error) { return message.Clone(msg), err },
	)
}

func main() {
	ctx := context.Background()

	rawDeployTool, err := functiontool.New(functiontool.Config{
		Name:        "deploy_service",
		Description: "Submits a deployment request to an external deployment API.",
	}, func(ctx adktool.Context, args deployArgs) (deployResult, *adktool.Control, error) {
		// This is where a real implementation would call your deployment API.
		return deployResult{
			OperationID: fmt.Sprintf("deploy-%s-%s", args.Service, args.Version),
			Status:      "submitted",
			Service:     args.Service,
			Version:     args.Version,
		}, nil, nil
	})
	if err != nil {
		log.Fatalf("create raw deploy tool: %v", err)
	}

	approvalWrapped := interrupt.WithApproval(rawDeployTool, "Approve this deployment before the API request is submitted.")

	hostPort := getenvDefault("TEMPORAL_HOSTPORT", "localhost:7233")
	taskQueue := getenvDefault("TEMPORAL_TASK_QUEUE", "adk-temporal-tools-example")
	temporalClientOptions := client.Options{HostPort: hostPort}

	sessionSvc := session.InMemoryService()

	worker, err := toolworker.New(toolworker.Config{
		ClientOptions:  temporalClientOptions,
		TaskQueue:      taskQueue,
		SessionService: sessionSvc,
	})
	if err != nil {
		log.Fatalf("create temporal tool worker: %v", err)
	}
	if err := worker.Start(); err != nil {
		log.Fatalf("start temporal tool worker: %v", err)
	}
	defer worker.Close()

	durableDeployTool, err := temporaltool.New(temporaltool.Config{
		Wrapped: approvalWrapped,
		Worker:  worker,
	})
	if err != nil {
		log.Fatalf("create durable temporal tool: %v", err)
	}

	root, err := llmagent.New(llmagent.Config{
		Name:        "release_manager",
		Description: "Submits deployment requests with approval.",
		Model: &scriptedModel{
			responses: []*message.Message{
				message.NewMessage(message.RoleAssistant, []message.Content{
					&message.ToolCallContent{
						ID:   "call-deploy",
						Name: durableDeployTool.Name(),
						Arguments: map[string]any{
							"service": "billing-api",
							"version": "2026.04.22",
						},
					},
				}),
				message.NewAssistantMessage("Deployment approved. The request was submitted to the deployment API."),
			},
		},
		ModelID:     "scripted",
		Instruction: "Use the deployment tool when the user asks to deploy a service.",
		Tools:       []adktool.Tool{durableDeployTool},
	})
	if err != nil {
		log.Fatalf("create root agent: %v", err)
	}

	const (
		appName   = "temporaltool-example"
		userID    = "demo-user"
		sessionID = "demo-session"
	)

	if _, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}); err != nil {
		log.Fatalf("create session: %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          root,
		SessionService: sessionSvc,
	})
	if err != nil {
		log.Fatalf("create runner: %v", err)
	}

	fmt.Printf("Temporal server: %s\n", hostPort)
	fmt.Printf("Temporal task queue: %s\n\n", taskQueue)

	fmt.Println("=== First run ===")
	intr, err := runAndPrint(ctx, r, userID, sessionID, message.NewUserMessage("Please deploy billing-api version 2026.04.22."), agent.RunOptions{})
	if err != nil {
		log.Fatalf("first run failed: %v", err)
	}
	if intr == nil {
		log.Fatalf("expected interrupt from durable tool, got none")
	}

	resumeData := map[string]any{
		"call-deploy": map[string]any{"approved": true},
	}
	fmt.Printf("\nResuming with: %#v\n\n", resumeData)

	fmt.Println("=== Resume run ===")
	if _, err := runAndPrint(ctx, r, userID, sessionID, nil, agent.RunOptions{
		ResumeData: resumeData,
	}); err != nil {
		log.Fatalf("resume run failed: %v", err)
	}
}

func runAndPrint(ctx context.Context, r *runner.Runner, userID, sessionID string, input *message.Message, opts agent.RunOptions) (*event.Interrupt, error) {
	var interruptEvent *event.Interrupt
	for ev, err := range r.Run(ctx, userID, sessionID, input, opts) {
		if err != nil {
			return nil, err
		}
		switch e := ev.(type) {
		case *event.MessageEnd:
			printMessage(e.Message)
		case *event.Interrupt:
			interruptEvent = e
			fmt.Println("interrupt:")
			for _, interruptCtx := range e.Interrupt.Contexts {
				fmt.Printf("  id=%s address=%s info=%#v\n", interruptCtx.ID, interruptCtx.Address, interruptCtx.Info)
			}
		case *event.ToolExecutionStart:
			fmt.Printf("tool start: %s args=%#v\n", e.ToolName, e.Args)
		case *event.ToolExecutionEnd:
			fmt.Printf("tool end: %s result=%#v\n", e.ToolName, e.Result)
		}
	}
	return interruptEvent, nil
}

func printMessage(msg *message.Message) {
	if msg == nil {
		return
	}

	switch msg.Role {
	case message.RoleUser:
		fmt.Printf("user: %s\n", msg.Text())
	case message.RoleAssistant:
		if calls := msg.ToolCalls(); len(calls) > 0 {
			for _, call := range calls {
				fmt.Printf("assistant requested tool: %s args=%#v\n", call.Name, call.Args)
			}
			return
		}
		if text := msg.Text(); text != "" {
			fmt.Printf("assistant: %s\n", text)
		}
	case message.RoleToolResult:
		for _, result := range msg.ToolResults() {
			fmt.Printf("tool result: %s => %#v\n", result.Name, result.Content)
		}
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
