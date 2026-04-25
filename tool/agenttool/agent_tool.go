// Package agenttool provides a tool that allows an agent to call another agent.
// This enables composition of agents, which can be useful for scenarios where
// different types of `genai` tools cannot be used together.
package agenttool

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/llminternal"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

// agentTool implements a tool that allows an agent to call another agent.
type agentTool struct {
	agent             agent.Agent
	skipSummarization bool
	sessionService    session.Service // Persists across calls to support resume

	runner     *runner.Runner
	runnerOnce sync.Once
	runnerErr  error
}

// Config holds the configuration for an agent tool.
type Config struct {
	// SkipSummarization, if true, will cause the agent to skip summarization
	// after the sub-agent finishes execution.
	SkipSummarization bool
	// SessionService persists sub-agent sessions across calls and resumes.
	// AgentTool requires an explicit service so callers control runtime storage.
	SessionService session.Service
}

// New creates a new agent tool.
// If cfg is nil, zero-value behavior is used and Execute will fail until
// SessionService is provided.
func New(agent agent.Agent, cfg *Config) tool.Tool {
	if cfg == nil {
		return &agentTool{
			agent:             agent,
			skipSummarization: false,
		}
	}
	return &agentTool{
		agent:             agent,
		skipSummarization: cfg.SkipSummarization,
		sessionService:    cfg.SessionService,
	}
}

// Name implements tool.Tool.
func (t *agentTool) Name() string {
	return t.agent.Name()
}

// Description implements tool.Tool.
func (t *agentTool) Description() string {
	return t.agent.Description()
}

func (t *agentTool) Schema() map[string]any {
	var agentInputSchema map[string]any
	llmAgent, ok := t.agent.(llminternal.SchemaCarrier)
	if ok && llmAgent != nil {
		agentInputSchema = llmAgent.InputSchemaConfig()
	}

	if agentInputSchema != nil {
		return agentInputSchema
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"request"},
	}
}

// executeSubAgent runs the wrapped agent and normalizes its final output.
func (t *agentTool) executeSubAgent(toolCtx tool.Context, margs map[string]any) (any, *tool.Control, error) {
	if t.sessionService == nil {
		return nil, nil, fmt.Errorf("agenttool requires Config.SessionService")
	}
	ctrl := &tool.Control{}
	if t.skipSummarization {
		ctrl.Terminal = true
	}

	var agentInputSchema map[string]any
	llmAgent, ok := t.agent.(llminternal.SchemaCarrier)
	isLllmAgent := ok && llmAgent != nil
	if isLllmAgent {
		agentInputSchema = llmAgent.InputSchemaConfig()
	}

	var inputMsg *message.Message
	if agentInputSchema != nil {
		jsonData, err := json.Marshal(margs)
		if err != nil {
			return nil, ctrl, fmt.Errorf("error serializing tool arguments for agent %s: %w", t.agent.Name(), err)
		}
		inputMsg = message.NewUserMessage(string(jsonData))
	} else {
		input, ok := margs["request"]
		if !ok {
			return nil, ctrl, fmt.Errorf("missing required argument 'request' for agent %s", t.agent.Name())
		}
		inputText, ok := input.(string)
		if !ok {
			inputText = fmt.Sprint(input)
		}
		inputMsg = message.NewUserMessage(inputText)
	}

	r, err := t.getRunner()
	if err != nil {
		return nil, ctrl, err
	}

	// Inherit runtime behavior from the parent call rather than forcing
	// transport/storage defaults inside the tool itself.
	runCfg := inheritedRunOptions(toolCtx)
	isResumeTarget, hasData, resumeData := toolCtx.ResumeData()
	if isResumeTarget && hasData {
		if rd, ok := resumeData.(map[string]any); ok {
			runCfg.ResumeData = rd
		}
	}

	// Check if this is a resume call - restore previous session if so
	var subSession session.Session
	if hasState, state := toolCtx.InterruptState(); hasState {
		// Resume: get the previous session by ID
		if stateMap, ok := state.(map[string]any); ok {
			if sessionID, ok := stateMap["session_id"].(string); ok {
				resp, err := t.sessionService.Get(toolCtx.Context(), &session.GetRequest{
					AppName:   t.agent.Name(),
					UserID:    toolCtx.InvocationContext().Session().UserID(),
					SessionID: sessionID,
				})
				if err != nil {
					return nil, ctrl, fmt.Errorf("failed to get session for resume: %w", err)
				}
				subSession = resp.Session
			}
		}
	}

	// If not resume (or resume failed to find session), create new session
	if subSession == nil {
		resp, err := t.sessionService.Create(toolCtx.Context(), &session.CreateRequest{
			AppName: t.agent.Name(),
			UserID:  toolCtx.InvocationContext().Session().UserID(),
		})
		if err != nil {
			return nil, ctrl, fmt.Errorf("failed to create session for sub-agent %s: %w", t.agent.Name(), err)
		}
		subSession = resp.Session
	}

	// TODO(dpasiukevich): verify agent loop termination.
	// For resume, pass nil content to continue from where we left off
	var runInput *message.Message
	if !isResumeTarget {
		runInput = inputMsg
	}
	eventCh := r.Run(toolCtx.Context(), subSession.UserID(), subSession.ID(), runInput, runCfg)

	var lastMessage *message.Message
	for output, err := range eventCh {
		if err != nil {
			return nil, ctrl, fmt.Errorf("error during execution of sub-agent %s: %w", t.agent.Name(), err)
		}
		if output == nil {
			continue
		}
		// Check for interrupt from sub-agent and propagate it.
		if intr, ok := output.(*event.Interrupt); ok && intr.Interrupt != nil {
			subSigs := intr.Interrupt.ToSignals()
			address := fmt.Sprintf("%s:agenttool:%s", toolCtx.InvocationContext().Address(), t.agent.Name())
			// Save session ID in state so we can restore it on resume
			state := map[string]any{"session_id": subSession.ID()}
			compositeSig := resume.CompositeInterrupt(address, "agent tool interrupt", state, subSigs...)
			return nil, ctrl, compositeSig
		}
		if msgEnd, ok := output.(*event.MessageEnd); ok && msgEnd.Message != nil && msgEnd.Message.Role == message.RoleAssistant {
			lastMessage = message.Clone(msgEnd.Message)
		}
	}

	if lastMessage == nil {
		return map[string]any{}, ctrl, nil
	}

	outputText := lastMessage.Text()

	if outputText == "" {
		return map[string]any{}, ctrl, nil
	}
	if isLllmAgent {
		if agentOutputSchema := llmAgent.OutputSchemaConfig(); agentOutputSchema != nil {
			parsedOutput, err := validateOutputSchema(outputText, agentOutputSchema)
			if err != nil {
				return nil, ctrl, fmt.Errorf("output validation failed for sub-agent %s: %w", t.agent.Name(), err)
			}
			return parsedOutput, ctrl, nil
		}
	}

	return map[string]any{"result": outputText}, ctrl, nil
}

func (t *agentTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	return t.executeSubAgent(ctx, args)
}

func (t *agentTool) getRunner() (*runner.Runner, error) {
	t.runnerOnce.Do(func() {
		if t.sessionService == nil {
			t.runnerErr = fmt.Errorf("agenttool requires Config.SessionService")
			return
		}
		t.runner, t.runnerErr = runner.New(runner.Config{
			AppName:        t.agent.Name(),
			Agent:          t.agent,
			SessionService: t.sessionService,
		})
	})
	return t.runner, t.runnerErr
}

func inheritedRunOptions(ctx tool.Context) agent.RunOptions {
	var cfg agent.RunOptions
	invCtx := ctx.InvocationContext()
	if invCtx == nil {
		return cfg
	}
	parentCfg := invCtx.RunConfig()
	parentState := invCtx.RunState()
	cfg.StreamingMode = parentCfg.StreamingMode
	if len(parentCfg.Extensions) > 0 {
		cfg.Extensions = append([]agent.Extension(nil), parentCfg.Extensions...)
	}
	cfg.AgentStateService = parentState.AgentStateService
	return cfg
}

func validateOutputSchema(outputText string, schema map[string]any) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(outputText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse output as JSON: %w", err)
	}
	return result, nil
}
