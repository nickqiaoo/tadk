package stream

import (
	"fmt"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
)

// StreamState tracks UI stream state across partial responses so
// that text/reasoning deltas and block boundaries are emitted correctly.
// Create one per agent run and pass it to EntryToParts on each entry.
type StreamState struct {
	// Whether we have emitted Start for this message stream.
	messageStarted bool
	// Whether we have emitted StartStep for the current step.
	stepStarted bool

	// Text block tracking
	textID      string
	textStarted bool
	prevText    string

	// Reasoning block tracking
	reasoningID      string
	reasoningStarted bool
	prevThinking     string

	// Counter for generating unique block IDs.
	nextID int

	// Tool call tracking to avoid emitting the same call multiple times when a
	// provider first surfaces it in a partial chunk and then repeats it in the
	// final assistant message.
	emittedToolCalls  map[string]bool
	startedToolCalls  map[string]bool
	textBlockIDs      map[int]string
	reasoningBlockIDs map[int]string
}

// NewStreamState creates a fresh StreamState for a new agent run.
func NewStreamState() *StreamState {
	return &StreamState{
		emittedToolCalls:  make(map[string]bool),
		startedToolCalls:  make(map[string]bool),
		textBlockIDs:      make(map[int]string),
		reasoningBlockIDs: make(map[int]string),
	}
}

func (s *StreamState) genID(prefix string) string {
	id := fmt.Sprintf("%s_%d", prefix, s.nextID)
	s.nextID++
	return id
}

// EntryToParts converts a durable session entry into a sequence of UI Message
// Stream Protocol chunks. It tracks streaming state across calls so that
// text/reasoning deltas and block boundaries are computed correctly.
//
// Returns nil if the entry produces no output.
func EntryToParts(entry session.DurableEntry, state *StreamState) []Part {
	if entry == nil {
		return nil
	}
	msgEntry, ok := entry.(*session.MessageEntry)
	if !ok {
		return nil
	}
	return entryToParts(msgEntry, state)
}

func entryToParts(entry *session.MessageEntry, state *StreamState) []Part {
	if entry == nil {
		return nil
	}

	var parts []Part

	// Handle error events (no message content)
	if entry.ErrorMessage != "" && entry.Message == nil {
		return []Part{Error{ErrorText: entry.ErrorMessage}}
	}

	if entry.Message == nil {
		return parts
	}

	baseID := ""
	if b := entry.Base(); b != nil {
		baseID = b.ID
	}

	// Emit Start once per message stream (before first assistant event)
	if entry.Message.Role == message.RoleAssistant && !state.messageStarted {
		state.messageStarted = true
		msgID := baseID
		if msgID == "" {
			msgID = "msg"
		}
		parts = append(parts, Start{MessageID: msgID})
	}

	// Emit StartStep at beginning of each new step (LLM call)
	if entry.Message.Role == message.RoleAssistant && !state.stepStarted {
		state.stepStarted = true
		state.prevText = ""
		state.prevThinking = ""
		state.textStarted = false
		state.reasoningStarted = false
		parts = append(parts, StartStep{})
	}

	// Process message content
	if entry.Message.Role == message.RoleAssistant {
		parts = append(parts, assistantParts(entry, state)...)
		parts = append(parts, assistantToolCallParts(entry.Message, state)...)
	} else if entry.Message.Role == message.RoleToolResult {
		parts = append(parts, toolParts(entry.Message)...)
	}

	// Close blocks + emit step/message end for completed assistant responses
	if entry.Message.Role == message.RoleAssistant {
		// Close reasoning block first
		if state.reasoningStarted {
			endPart := ReasoningEnd{ID: state.reasoningID}
			for _, p := range entry.Message.ThinkingContents() {
				if tc, ok := p.(*message.ThinkingContent); ok && tc.ThinkingSignature != "" {
					meta := map[string]any{"signature": tc.ThinkingSignature}
					if tc.Redacted {
						meta["redacted"] = true
					}
					endPart.ProviderMetadata = map[string]any{"anthropic": meta}
					break
				}
			}
			parts = append(parts, endPart)
			state.reasoningStarted = false
		}

		// Close text block
		if state.textStarted {
			parts = append(parts, TextEnd{ID: state.textID})
			state.textStarted = false
		}

		hasToolCalls := entry.Message.HasToolCalls()
		parts = append(parts, FinishStep{})

		// Finish message only when the agent is truly done (no more tool calls)
		if !hasToolCalls {
			parts = append(parts, Finish{
				FinishReason:    mapStopReason(entry.StopReason),
				MessageMetadata: usageMetadata(entry.Usage),
			})
		}

		// Reset step state for the next turn
		state.stepStarted = false
	}

	return parts
}

func assistantEventToParts(ev event.Event, state *StreamState) []Part {
	if ev == nil {
		return nil
	}
	var parts []Part
	switch e := ev.(type) {
	case *event.MessageStart:
		if !state.messageStarted {
			state.messageStarted = true
			parts = append(parts, Start{MessageID: "msg"})
		}
		if !state.stepStarted {
			state.stepStarted = true
			parts = append(parts, StartStep{})
		}
	case *event.TextDelta:
		idx := e.ContentIndex
		id := state.textBlockIDs[idx]
		if id == "" {
			id = state.genID("text")
			state.textBlockIDs[idx] = id
			parts = append(parts, TextStart{ID: id})
		}
		parts = append(parts, TextDelta{ID: id, Delta: e.Delta})
	case *event.ThinkingDelta:
		idx := e.ContentIndex
		id := state.reasoningBlockIDs[idx]
		if id == "" {
			id = state.genID("reasoning")
			state.reasoningBlockIDs[idx] = id
			parts = append(parts, ReasoningStart{ID: id})
		}
		parts = append(parts, ReasoningDelta{ID: id, Delta: e.Delta})
	case *event.ToolCallDelta:
		key := e.ToolCallID
		if key == "" {
			key = fmt.Sprintf("%s:%d", e.ToolName, e.ContentIndex)
		}
		if !state.startedToolCalls[key] {
			state.startedToolCalls[key] = true
			parts = append(parts, ToolInputStart{ToolCallID: e.ToolCallID, ToolName: e.ToolName})
		}
		if e.Delta != "" {
			parts = append(parts, ToolInputDelta{ToolCallID: e.ToolCallID, InputTextDelta: e.Delta})
		}
	case *event.MessageEnd:
		if e.Message != nil {
			for i, tc := range e.Message.ToolCalls() {
				key := tc.ID
				if key == "" {
					key = fmt.Sprintf("%s:%d", tc.Name, i)
				}
				if !state.startedToolCalls[key] {
					state.startedToolCalls[key] = true
					parts = append(parts, ToolInputStart{
						ToolCallID: tc.ID,
						ToolName:   tc.Name,
					})
				}
				if state.emittedToolCalls[key] {
					continue
				}
				state.emittedToolCalls[key] = true
				parts = append(parts, ToolInputAvailable{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Input:      tc.Args,
				})
			}
		}
		// Close reasoning block first
		if state.reasoningStarted {
			endPart := ReasoningEnd{ID: state.reasoningID}
			for _, p := range e.Message.ThinkingContents() {
				if tc, ok := p.(*message.ThinkingContent); ok && tc.ThinkingSignature != "" {
					meta := map[string]any{"signature": tc.ThinkingSignature}
					if tc.Redacted {
						meta["redacted"] = true
					}
					endPart.ProviderMetadata = map[string]any{"anthropic": meta}
					break
				}
			}
			parts = append(parts, endPart)
			state.reasoningStarted = false
		}
		// Close text block
		if state.textStarted {
			parts = append(parts, TextEnd{ID: state.textID})
			state.textStarted = false
		}
		hasToolCalls := e.Message != nil && e.Message.HasToolCalls()
		parts = append(parts, FinishStep{})
		if !hasToolCalls {
			parts = append(parts, Finish{
				FinishReason:    mapStopReason(e.Message.StopReason),
				MessageMetadata: usageMetadata(nilIfNoMessage(e.Message)),
			})
		}
		state.stepStarted = false
	}
	return parts
}



func nilIfNoMessage(msg *message.Message) *model.Usage {
	if msg == nil {
		return nil
	}
	return msg.Usage
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func assistantToolCallParts(msg *message.Message, state *StreamState) []Part {
	if msg == nil {
		return nil
	}
	var parts []Part
	for i, tc := range msg.ToolCalls() {
		key := tc.ID
		if key == "" {
			key = fmt.Sprintf("%s:%d", tc.Name, i)
		}
		if !state.startedToolCalls[key] {
			state.startedToolCalls[key] = true
			parts = append(parts, ToolInputStart{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			})
		}
		if state.emittedToolCalls[key] {
			continue
		}
		state.emittedToolCalls[key] = true
		parts = append(parts, ToolInputAvailable{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Input:      tc.Args,
		})
	}
	return parts
}

func metadataParts(metadata map[string]any, state *StreamState) []Part {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["toolInputDelta"]
	if !ok {
		return nil
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	toolCallID, _ := payload["toolCallId"].(string)
	toolName, _ := payload["toolName"].(string)
	delta, _ := payload["delta"].(string)
	if toolCallID == "" || delta == "" {
		return nil
	}
	key := toolCallID
	if !state.startedToolCalls[key] {
		state.startedToolCalls[key] = true
		return []Part{
			ToolInputStart{
				ToolCallID: toolCallID,
				ToolName:   toolName,
			},
			ToolInputDelta{
				ToolCallID:     toolCallID,
				InputTextDelta: delta,
			},
		}
	}
	return []Part{
		ToolInputDelta{
			ToolCallID:     toolCallID,
			InputTextDelta: delta,
		},
	}
}

// assistantParts extracts text/reasoning deltas from an assistant message.
func assistantParts(entry *session.MessageEntry, state *StreamState) []Part {
	var parts []Part
	msg := entry.Message

	// Accumulate current text and thinking
	curText := ""
	curThinking := ""
	for _, p := range msg.TextContents() {
		if tc, ok := p.(*message.TextContent); ok {
			curText += tc.Text
		}
	}
	for _, p := range msg.ThinkingContents() {
		if tc, ok := p.(*message.ThinkingContent); ok {
			curThinking += tc.Thinking
		}
	}

	// Reasoning delta (emit before text)
	if len(curThinking) > len(state.prevThinking) {
		if !state.reasoningStarted {
			state.reasoningID = state.genID("reasoning")
			state.reasoningStarted = true
			parts = append(parts, ReasoningStart{ID: state.reasoningID})
		}
		delta := curThinking[len(state.prevThinking):]
		parts = append(parts, ReasoningDelta{ID: state.reasoningID, Delta: delta})
		state.prevThinking = curThinking
	}

	// Text delta
	if len(curText) > len(state.prevText) {
		if !state.textStarted {
			state.textID = state.genID("text")
			state.textStarted = true
			parts = append(parts, TextStart{ID: state.textID})
		}
		delta := curText[len(state.prevText):]
		parts = append(parts, TextDelta{ID: state.textID, Delta: delta})
		state.prevText = curText
	}

	return parts
}

// toolParts extracts tool results from a tool message.
func toolParts(msg *message.Message) []Part {
	var parts []Part
	for _, tr := range msg.ToolResults() {
		if tr.IsError {
			parts = append(parts, ToolOutputError{
				ToolCallID: tr.CallID,
				ErrorText:  fmt.Sprintf("%v", tr.Content),
			})
		} else {
			parts = append(parts, ToolOutputAvailable{
				ToolCallID: tr.CallID,
				Output:     tr.Content,
			})
		}
	}
	return parts
}

// mapStopReason converts model.StopReason to Vercel FinishReason.
func mapStopReason(r model.StopReason) FinishReason {
	switch r {
	case model.StopReasonStop:
		return FinishReasonStop
	case model.StopReasonLength:
		return FinishReasonLength
	case model.StopReasonToolUse:
		return FinishReasonToolCalls
	case model.StopReasonError, model.StopReasonAborted:
		return FinishReasonError
	case model.StopReasonSafety:
		return FinishReasonContentFilter
	default:
		return FinishReasonUnknown
	}
}

// usageMetadata wraps usage info in messageMetadata format.
func usageMetadata(u *model.Usage) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"usage": map[string]any{
			"promptTokens":     u.InputTokens,
			"completionTokens": u.OutputTokens,
		},
	}
}
