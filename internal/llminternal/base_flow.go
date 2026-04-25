package llminternal

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/llminternal/processors"
	"github.com/nickqiaoo/tadk/internal/telemetry"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/internal/utils"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/resume"

	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

var ErrModelNotConfigured = errors.New("model not configured; ensure Model is set in llmagent.Config")
var ErrModelIDNotConfigured = errors.New("model id not configured; ensure llmagent.Config.ModelID or Request.Model is set")

type Flow struct {
	Model  model.ModelAdapter
	Config *Config
	Agent  agent.Agent
	Tree   agent.ParentTree

	// Prefix carries the immutable, invocation-scoped portion of the LLM
	// request (system prompt, tool schemas, model config). It is built once
	// in llmagent.run via BuildPrefix and reused across every Step so that
	// LLM-provider prompt caching can hit on the static prefix.
	Prefix *Prefix

	// ModelExts / ToolExts are the pre-filtered extension hooks for this
	// invocation. Populated once at Flow construction so every Step/tool call
	// can reuse them without re-scanning Extensions on each hook.
	ModelExts []agent.ModelExtension
	ToolExts  []agent.ToolExtension
}

type StepResult struct {
	Continue        bool
	Transferred     bool
	TransferToAgent string
}

// Step executes a single assistant/tool turn and materializes public
// AgentEvents. Events are pushed to yield in real time as they happen
// (model deltas, tool execution events, etc.). The StepResult is only
// available via the return value, after every event has been yielded —
// this makes the iter/result ordering compile-time enforced.
//
// Step does not own the surrounding agent loop. The caller is responsible
// for emitting turn_start/agent_start and deciding whether to continue
// with another step.
func (f *Flow) Step(ctx agent.InvocationContext, yield func(event.Event, error) bool) (*StepResult, error) {
	if state := ctx.RunState(); len(state.ResumeData) > 0 {
		resumeEvents, err := f.handleResumeCalls(ctx, state.ResumeData)
		if err != nil {
			yield(nil, err)
			return &StepResult{}, err
		}
		res, stopped := f.processPipeEvents(f.resumePipe(resumeEvents), yield)
		if stopped {
			return &StepResult{}, nil
		}
		r := deriveStepResult(res)
		return &r, nil
	}

	res, stopped := f.processPipeEvents(f.runStepItems(ctx), yield)
	if stopped {
		return &StepResult{}, nil
	}
	r := deriveStepResult(res)
	return &r, nil
}

// deriveStepResult maps a pipe's terminal state into the StepResult the
// agent loop acts on. Transfer wins over everything; otherwise the next step
// is dictated by LastMessage and Terminal.
func deriveStepResult(res *PipeResult) StepResult {
	if res == nil {
		return StepResult{}
	}
	if res.Control != nil && res.Control.TransferToAgent != "" {
		return StepResult{
			Transferred:     true,
			TransferToAgent: res.Control.TransferToAgent,
		}
	}
	if res.LastMessage == nil {
		return StepResult{}
	}
	if res.Control != nil && res.Control.Terminal {
		return StepResult{}
	}
	return StepResult{Continue: !isFinalResponse(res.LastMessage)}
}

// processPipeEvents drains a pipe's events through yield and returns its final state.
func (f *Flow) processPipeEvents(
	pipe EventPipe,
	yield func(event.Event, error) bool,
) (*PipeResult, bool) {
	for ev, err := range pipe.Events {
		if err != nil {
			yield(nil, err)
			return nil, true
		}
		if !yield(ev, nil) {
			return nil, true
		}
	}
	res, err := pipe.Finalize()
	if err != nil {
		yield(nil, err)
		return nil, true
	}
	return &res, false
}

// resumePipe wraps a slice of resume events into an EventPipe.
func (f *Flow) resumePipe(events []event.Event) EventPipe {
	return EventPipe{
		Events: func(yield func(event.Event, error) bool) {
			for _, ev := range events {
				if !yield(ev, nil) {
					return
				}
			}
		},
		Finalize: func() (PipeResult, error) {
			var res PipeResult
			for _, ev := range events {
				if msgEnd, ok := ev.(*event.MessageEnd); ok && msgEnd != nil {
					res.LastMessage = msgEnd.Message
				}
				if tev, ok := ev.(*event.ToolExecutionEnd); ok {
					ctrl := tev.Control()
					if res.Control == nil {
						res.Control = &tool.Control{}
					}
					if ctrl.TransferToAgent != "" {
						res.Control.TransferToAgent = ctrl.TransferToAgent
					}
					if ctrl.ExitToParent {
						res.Control.ExitToParent = true
					}
					if ctrl.Terminal {
						res.Control.Terminal = true
					}
				}
			}
			return res, nil
		},
	}
}

func isFinalResponse(msg *message.Message) bool {
	if msg == nil {
		return false
	}
	if msg.Role != message.RoleAssistant || msg.HasToolCalls() {
		return false
	}
	return true
}

func (f *Flow) handleResumeCalls(ctx agent.InvocationContext, resumeData map[string]any) ([]event.Event, error) {
	if state := ctx.RunState(); state.Checkpoint != nil {
		if entry, handled, err := f.handleResumeFromCheckpoint(ctx, resumeData, state.Checkpoint); handled || err != nil {
			return entry, err
		}
	}
	return nil, fmt.Errorf("no checkpoint found for resume")
}

func (f *Flow) handleResumeFromCheckpoint(ctx agent.InvocationContext, resumeData map[string]any, checkpoint *session.Checkpoint) ([]event.Event, bool, error) {
	if checkpoint == nil || len(checkpoint.PendingInterrupts) == 0 {
		return nil, false, nil
	}

	req := &model.Request{}
	toolsMap, err := f.preprocess(ctx, req)
	if err != nil {
		return nil, true, err
	}
	if req.Model == "" {
		if fallback := f.Model.Name(); fallback != "" {
			req.Model = fallback
		}
	}
	if req.Model == "" {
		return nil, true, ErrModelIDNotConfigured
	}

	type resumeResult struct {
		callID  string
		name    string
		args    map[string]any
		result  map[string]any
		isError bool
	}

	var results []resumeResult
	var toolResultContents []message.Content
	handledAny := false
	var transferToAgent string
	var exitToParent bool
	var terminal bool

	for _, pending := range checkpoint.PendingInterrupts {
		if pending == nil || pending.ToolCallID == "" || pending.ToolName == "" {
			continue
		}
		if pending.AgentAddress != "" && pending.AgentAddress != ctx.Address() {
			continue
		}
		data, ok := utils.ResumeDataForToolCall(resumeData, pending.ToolCallID, pending.ID)
		if !ok {
			return nil, true, fmt.Errorf("missing resume data for interrupt %s (tool call %s)", pending.ID, pending.ToolCallID)
		}

		curTool, ok := toolsMap[pending.ToolName]
		if !ok {
			return nil, true, fmt.Errorf("unknown tool: %q", pending.ToolName)
		}
		toolCtx := toolinternal.NewToolContext(ctx, pending.ToolCallID,
			toolinternal.WithResumeData(true, data),
			toolinternal.WithInterruptState(pending.State))

		result, toolCtrl, err := f.callTool(toolCtx, curTool, &message.ToolCall{
			ID:   pending.ToolCallID,
			Name: pending.ToolName,
			Args: pending.ToolArgs,
		})
		if err != nil {
			return nil, true, err
		}

		if toolCtrl == nil {
			toolCtrl = &tool.Control{}
		}
		if toolCtrl.TransferToAgent != "" {
			transferToAgent = toolCtrl.TransferToAgent
		}
		if toolCtrl.ExitToParent {
			exitToParent = true
		}
		if toolCtrl.Terminal {
			terminal = true
		}

		results = append(results, resumeResult{
			callID:  pending.ToolCallID,
			name:    pending.ToolName,
			args:    pending.ToolArgs,
			result:  result,
			isError: false,
		})
		toolResultContents = append(toolResultContents, message.NewToolResultMessage(pending.ToolCallID, result, false).Content...)
		handledAny = true
	}
	if !handledAny {
		return nil, false, nil
	}

	var toolResults []message.ToolResult
	for _, r := range results {
		toolResults = append(toolResults, message.ToolResult{
			CallID:  r.callID,
			Name:    r.name,
			Content: r.result,
			IsError: r.isError,
		})
	}

	ctrl := event.Control{
		TransferToAgent: transferToAgent,
		ExitToParent:    exitToParent,
		Terminal:        terminal,
	}

	msg := message.NewToolResultMessages(toolResults, toolResultContents)
	var evts []event.Event
	opts := event.WithControl(ctrl)
	for _, r := range results {
		evts = append(evts, event.NewToolExecutionStart(r.callID, r.name, r.args, opts))
	}
	evts = append(evts,
		event.NewMessageStart(msg, opts),
		event.NewMessageEnd(msg, opts),
	)
	for _, r := range results {
		evts = append(evts, event.NewToolExecutionEnd(r.callID, r.name, r.result, r.isError, opts))
	}
	return evts, true, nil
}

// runStepItems executes the model/tool pipeline for a single turn and emits
// internal flow items. It intentionally does not handle resume state,
// agent transfer, or AgentEvent materialization; Flow.Step owns that boundary.
func (f *Flow) runStepItems(ctx agent.InvocationContext) EventPipe {
	var (
		stepCtrl *tool.Control
		lastMsg  *message.Message
	)
	events := func(yield func(event.Event, error) bool) {
		if f.Model == nil {
			yield(nil, fmt.Errorf("agent %q: %w", ctx.AgentName(), ErrModelNotConfigured))
			return
		}

		req := &model.Request{}

		// Preprocess before calling the LLM.
		toolsMap, err := f.preprocess(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		if req.Model == "" {
			if fallback := f.Model.Name(); fallback != "" {
				req.Model = fallback
			}
		}
		if req.Model == "" {
			yield(nil, fmt.Errorf("agent %q: %w", ctx.AgentName(), ErrModelIDNotConfigured))
			return
		}
		// Call the LLM.
		var finalMsg *message.Message
		for ev, err := range f.callLLM(ctx, req) {
			if err != nil {
				yield(nil, err)
				return
			}
			if msgEnd, ok := ev.(*event.MessageEnd); ok && msgEnd != nil {
				finalMsg = msgEnd.Message
			}
			if !yield(ev, nil) {
				return
			}
		}

		if finalMsg != nil {
			message.EnsureToolCallIDs(finalMsg)
			// Handle tool calls
			toolEvents, ctrl, err := f.handleFunctionCalls(ctx, toolsMap, finalMsg)
			if err != nil {
				yield(nil, err)
				return
			}
			stepCtrl = ctrl
			lastMsg = finalMsg
			for _, tev := range toolEvents {
				if !yield(tev, nil) {
					return
				}
			}
		}
	}
	return EventPipe{
		Events: events,
		Finalize: func() (PipeResult, error) {
			return PipeResult{
				LastMessage: lastMsg,
				Control:     stepCtrl,
			}, nil
		},
	}
}

func (f *Flow) preprocess(ctx agent.InvocationContext, req *model.Request) (map[string]tool.Tool, error) {
	if f.Prefix == nil {
		return nil, fmt.Errorf("flow prefix not initialized; BuildPrefix must run before Step")
	}

	// Static prefix: identical byte-for-byte across every step of the
	// invocation. LLM-provider prompt caching relies on this. The shared
	// pointee of GenConfig must not be mutated by downstream adapters; see
	// model.Request.Config.
	req.Model = f.Prefix.ModelID
	req.Config = f.Prefix.GenConfig
	req.SystemPrompt = f.Prefix.SystemPrompt
	req.Tools = f.Prefix.ToolDefs

	// Dynamic tail: session history and related per-step messages.
	if f.Config != nil {
		if err := processors.Contents(ctx, req, f.Config); err != nil {
			return nil, err
		}
	}

	return f.Prefix.ToolsMap, nil
}

func (f *Flow) callLLM(ctx agent.InvocationContext, req *model.Request) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		shortCircuitMsg, err := f.runBeforeModelExtensions(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		if shortCircuitMsg != nil {
			yield(event.NewMessageEnd(shortCircuitMsg), nil)
			return
		}

		agentRunConfig := ctx.RunConfig()
		useStream := agentRunConfig.StreamingMode == agent.StreamingModeSSE

		var lastMsg *message.Message
		var lastErr error
		modelName := f.Model.Name()
		if req != nil && req.Model != "" {
			modelName = req.Model
		}
		spanCtx, span := telemetry.StartGenerateContentSpan(ctx.Context(), telemetry.StartGenerateContentSpanParams{
			ModelName:    modelName,
			InvocationID: ctx.InvocationID(),
		})
		llmCtx, cancel := context.WithCancel(spanCtx)
		defer cancel()
		ctx = ctx.WithContext(llmCtx)
		telemetry.LogRequest(ctx.Context(), req)
		defer func() {
			var eventID string
			if lastMsg != nil {
				eventID = uuid.New().String()
			}
			telemetry.TraceGenerateContentResult(span, telemetry.TraceGenerateContentResultParams{
				Message: lastMsg,
				EventID: eventID,
				Error:   lastErr,
			})
			span.End()
		}()

		if useStream {
			stream := f.Model.Stream(ctx.Context(), req)
			for ev, err := range stream.Events {
				if err != nil {
					lastErr = err
					yield(nil, err)
					return
				}
				if msgEnd, ok := ev.(*event.MessageEnd); ok && msgEnd != nil {
					lastMsg = msgEnd.Message
					telemetry.LogResponse(ctx.Context(), lastMsg)
					extMsg, extErr := f.runAfterModelExtensions(ctx, lastMsg, nil)
					if extErr != nil {
						yield(nil, extErr)
						return
					}
					if extMsg != nil {
						msgEnd.Message = extMsg
					}
				}
				if !yield(ev, nil) {
					return
				}
			}
			if final, err := stream.Result(); err != nil {
				lastErr = err
				yield(nil, err)
				return
			} else if final != nil {
				lastMsg = final
			}
			return
		}

		resp, err := f.Model.Generate(ctx.Context(), req)
		lastMsg = resp
		lastErr = err
		if err != nil {
			yield(nil, err)
			return
		}
		if resp != nil {
			telemetry.LogResponse(ctx.Context(), resp)
		}
		extMsg, extErr := f.runAfterModelExtensions(ctx, resp, err)
		if extErr != nil {
			yield(nil, extErr)
			return
		}
		if extMsg != nil {
			resp = extMsg
			err = nil
		}
		yield(event.NewMessageEnd(resp), err)
	}
}

type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string         { return f.name }
func (*fakeTool) Description() string    { return "Tool not found" }
func (*fakeTool) Schema() map[string]any { return nil }
func (*fakeTool) Execute(tool.Context, map[string]any) (any, *tool.Control, error) {
	return nil, nil, fmt.Errorf("tool not found")
}

var _ tool.Tool = (*fakeTool)(nil)

// newToolNotFoundError creates an error matching the specific Python format
func newToolNotFoundError(toolName string, availableTools []string) error {
	return fmt.Errorf("tool %q not found. Available tools: %v", toolName, availableTools)
}

// handleFunctionCalls calls the functions and returns the function response event
// plus an explicit control struct so callers do not need to scrape event.Control.
func (f *Flow) handleFunctionCalls(ctx agent.InvocationContext, toolsDict map[string]tool.Tool, msg *message.Message) ([]event.Event, *tool.Control, error) {
	if msg == nil {
		return nil, nil, nil
	}
	fnCalls := msg.ToolCalls()
	if len(fnCalls) == 0 {
		return nil, nil, nil
	}

	toolNames := make([]string, 0, len(toolsDict))
	for k := range toolsDict {
		toolNames = append(toolNames, k)
	}

	// Merged span for parallel tool calls
	if len(fnCalls) > 1 {
		mergedCtx, mergedToolCallSpan := telemetry.StartTrace(ctx.Context(), "execute_tool (merged)")
		ctx = ctx.WithContext(mergedCtx)
		defer func() {
			telemetry.TraceMergedToolCallsResult(mergedToolCallSpan, nil, nil)
			mergedToolCallSpan.End()
		}()
	}

	// Pre-allocate index-based slices so each goroutine writes to its own slot.
	fnResponseEvents := make([][]event.Event, len(fnCalls))
	sigSlots := make([]*resume.InterruptSignal, len(fnCalls))
	ctrlSlots := make([]*tool.Control, len(fnCalls))

	execToolCall := func(i int, fnCall message.ToolCall) error {
		sctx, span := telemetry.StartExecuteToolSpan(ctx.Context(), telemetry.StartExecuteToolSpanParams{
			ToolName: fnCall.Name,
			Args:     fnCall.Args,
		})
		defer span.End()
		toolCallCtx := ctx.WithContext(sctx)

		toolCtx := toolinternal.NewToolContext(toolCallCtx, fnCall.ID)

		var result map[string]any
		var toolCtrl *tool.Control
		curTool, found := toolsDict[fnCall.Name]
		if !found {
			result = map[string]any{"error": newToolNotFoundError(fnCall.Name, toolNames).Error()}
		} else {
			call := fnCall
			var callErr error
			result, toolCtrl, callErr = f.callTool(toolCtx, curTool, &call)
			if resume.IsInterrupt(callErr) {
				var sig *resume.InterruptSignal
				if errors.As(callErr, &sig) {
					sigSlots[i] = sig
				}
				ctrlSlots[i] = toolCtrl
				return nil
			}
			if callErr != nil {
				result = map[string]any{"error": callErr.Error()}
			}
			fnCall = call
		}
		if toolCtrl == nil {
			toolCtrl = &tool.Control{}
		}
		ctrlSlots[i] = toolCtrl
		isErrorResult := false
		if _, ok := result["error"]; ok {
			isErrorResult = true
		}
		resultPart := utils.ToolResult(utils.ToolCall{
			ID:   fnCall.ID,
			Name: fnCall.Name,
			Args: fnCall.Args,
		}, result, isErrorResult)
		msg := message.NewToolResultMessages([]message.ToolResult{resultPart}, message.NewToolResultMessage(resultPart.CallID, resultPart.Content, resultPart.IsError).Content)

		opts := event.WithControl(event.Control{
			TransferToAgent: toolCtrl.TransferToAgent,
			ExitToParent:    toolCtrl.ExitToParent,
			Terminal:        toolCtrl.Terminal,
		})
		evts := []event.Event{
			event.NewToolExecutionStart(fnCall.ID, fnCall.Name, fnCall.Args, opts),
			event.NewMessageStart(msg, opts),
			event.NewMessageEnd(msg, opts),
			event.NewToolExecutionEnd(fnCall.ID, fnCall.Name, result, isErrorResult, opts),
		}

		traceTool := curTool
		if traceTool == nil {
			traceTool = &fakeTool{name: fnCall.Name}
		}
		var toolErr error
		if errVal, ok := result["error"]; ok {
			if e, ok := errVal.(error); ok {
				toolErr = e
			} else if s, ok := errVal.(string); ok {
				toolErr = fmt.Errorf("%s", s)
			}
		}
		telemetryEntry := session.NewMessageLogEntry("", msg, ctx.AgentName(), ctx.InvocationID(), ctx.Branch())
		telemetry.TraceToolResult(span, telemetry.TraceToolResultParams{
			Description:   traceTool.Description(),
			ResponseEvent: telemetryEntry,
			Error:         toolErr,
		})

		fnResponseEvents[i] = evts
		return nil
	}

	if len(fnCalls) == 1 {
		// Single tool call: execute directly without goroutine overhead.
		if err := execToolCall(0, fnCalls[0]); err != nil {
			return nil, nil, err
		}
	} else {
		// Multiple tool calls: execute in parallel.
		g := new(errgroup.Group)
		for i, fnCall := range fnCalls {
			g.Go(func() error {
				return execToolCall(i, fnCall)
			})
		}
		if err := g.Wait(); err != nil {
			return nil, nil, err
		}
	}

	// Collect non-nil results.
	var allEvents []event.Event
	var sigs []*resume.InterruptSignal
	mergedCtrl := &tool.Control{}
	for i := range fnCalls {
		allEvents = append(allEvents, fnResponseEvents[i]...)
		if sigSlots[i] != nil {
			sigs = append(sigs, sigSlots[i])
		}
		if c := ctrlSlots[i]; c != nil {
			if c.TransferToAgent != "" {
				mergedCtrl.TransferToAgent = c.TransferToAgent
			}
			if c.ExitToParent {
				mergedCtrl.ExitToParent = true
			}
			if c.Terminal {
				mergedCtrl.Terminal = true
			}
		}
	}

	if len(sigs) > 0 {
		composeSig := resume.CompositeInterrupt(ctx.Address(), "composite interrupt from parallel function calls", nil, sigs...)
		allEvents = append(allEvents, event.NewInterrupt(composeSig.ToInterruptData()))
	}
	return allEvents, mergedCtrl, nil
}

func (f *Flow) callTool(toolCtx tool.Context, t tool.Tool, call *message.ToolCall) (map[string]any, *tool.Control, error) {
	var response map[string]any
	var ctrl *tool.Control
	var err error
	if call == nil {
		call = &message.ToolCall{}
	}
	if call.ID == "" {
		call.ID = toolCtx.FunctionCallID()
	}
	if call.Name == "" && t != nil {
		call.Name = t.Name()
	}

	isResume, _, _ := toolCtx.ResumeData()

	if !isResume {
		extResult, extErr := f.runBeforeToolExtensions(toolCtx, call)
		if extResult != nil || extErr != nil {
			response = toolResultContentMap(extResult)
			err = extErr
		}
	}

	if response == nil && err == nil {
		var output any
		output, ctrl, err = t.Execute(toolCtx, call.Args)
		if outMap, ok := output.(map[string]any); ok {
			response = outMap
		} else if output != nil {
			response = map[string]any{"result": output}
		}
	}
	if resume.IsInterrupt(err) {
		if ctrl == nil {
			ctrl = &tool.Control{}
		}
		return nil, ctrl, err
	}

	currentResult := &message.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: response,
		IsError: err != nil,
	}
	extResult, extErr := f.runAfterToolExtensions(toolCtx, call, currentResult, err)
	if extResult != nil || extErr != nil {
		response = toolResultContentMap(extResult)
		err = extErr
	}

	if err != nil {
		return map[string]any{"error": err.Error()}, ctrl, nil
	}
	return response, ctrl, nil
}

func (f *Flow) runBeforeModelExtensions(ctx agent.InvocationContext, req *model.Request) (*message.Message, error) {
	if ctx == nil {
		return nil, nil
	}
	var result *message.Message
	err := agent.ForEach(f.ModelExts, func(ext agent.ModelExtension) (bool, error) {
		msg, err := ext.BeforeModelCall(ctx, req)
		if msg != nil || err != nil {
			if err != nil {
				return true, fmt.Errorf("extension %q before model call failed: %w", ext.Name(), err)
			}
			result = msg
			return true, nil
		}
		return false, nil
	})
	return result, err
}

func (f *Flow) runAfterModelExtensions(ctx agent.InvocationContext, msg *message.Message, llmErr error) (*message.Message, error) {
	if ctx == nil {
		return nil, llmErr
	}
	if err := agent.ForEach(f.ModelExts, func(ext agent.ModelExtension) (bool, error) {
		replacement, err := ext.AfterModelCall(ctx, msg, llmErr)
		if replacement != nil || err != nil {
			if err != nil {
				return true, fmt.Errorf("extension %q after model call failed: %w", ext.Name(), err)
			}
			msg = replacement
			llmErr = nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return msg, llmErr
}

func (f *Flow) runBeforeToolExtensions(toolCtx tool.Context, call *message.ToolCall) (*message.ToolResult, error) {
	if toolCtx == nil || call == nil {
		return nil, nil
	}
	invocationCtx := toolCtx.InvocationContext()
	if invocationCtx == nil {
		return nil, nil
	}
	var result *message.ToolResult
	err := agent.ForEach(f.ToolExts, func(ext agent.ToolExtension) (bool, error) {
		res, err := ext.BeforeToolCall(invocationCtx, call)
		if res != nil || err != nil {
			if err != nil {
				return true, fmt.Errorf("extension %q before tool call failed: %w", ext.Name(), err)
			}
			result = res
			return true, nil
		}
		return false, nil
	})
	return result, err
}

func (f *Flow) runAfterToolExtensions(toolCtx tool.Context, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
	if toolCtx == nil || call == nil {
		return result, toolErr
	}
	invocationCtx := toolCtx.InvocationContext()
	if invocationCtx == nil {
		return result, toolErr
	}
	if err := agent.ForEach(f.ToolExts, func(ext agent.ToolExtension) (bool, error) {
		replacement, err := ext.AfterToolCall(invocationCtx, call, result, toolErr)
		if replacement != nil || err != nil {
			if err != nil {
				return true, fmt.Errorf("extension %q after tool call failed: %w", ext.Name(), err)
			}
			result = replacement
			toolErr = nil
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return result, toolErr
}

func toolResultContentMap(result *message.ToolResult) map[string]any {
	if result == nil {
		return nil
	}
	if content, ok := result.Content.(map[string]any); ok {
		return content
	}
	if result.Content == nil {
		return nil
	}
	return map[string]any{"result": result.Content}
}
