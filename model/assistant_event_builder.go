package model

import (
	"encoding/json"
	"fmt"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
)

type AssistantEventBuilder struct {
	partial *message.Message
	started bool

	currentTextIndex     int
	currentThinkingIndex int
	toolCalls            map[string]*toolCallState
	toolCallsByIndex     map[int]*toolCallState
}

func NewAssistantEventBuilder() *AssistantEventBuilder {
	return &AssistantEventBuilder{
		partial:              &message.Message{Role: message.RoleAssistant},
		currentTextIndex:     -1,
		currentThinkingIndex: -1,
		toolCalls:            make(map[string]*toolCallState),
		toolCallsByIndex:     make(map[int]*toolCallState),
	}
}

func (b *AssistantEventBuilder) PartialMessage() *message.Message {
	if b == nil {
		return nil
	}
	return message.Clone(b.partial)
}

func (b *AssistantEventBuilder) Start() event.Event {
	if b.started {
		return nil
	}
	b.started = true
	return event.NewMessageStart(message.Clone(b.partial))
}

func (b *AssistantEventBuilder) TextDelta(delta string) []event.Event {
	if delta == "" {
		return nil
	}
	var events []event.Event
	if ev := b.Start(); ev != nil {
		events = append(events, ev)
	}
	if b.currentTextIndex < 0 {
		b.currentTextIndex = len(b.partial.Content)
		b.partial.Content = append(b.partial.Content, &message.TextContent{})
	}
	cur := b.partial.Content[b.currentTextIndex].(*message.TextContent)
	cur.Text += delta
	b.partial.Content[b.currentTextIndex] = cur
	events = append(events, event.NewTextDelta(b.currentTextIndex, delta, message.Clone(b.partial)))
	return events
}

func (b *AssistantEventBuilder) ThinkingDelta(delta, signature string, redacted bool) []event.Event {
	if delta == "" && signature == "" && !redacted {
		return nil
	}
	var events []event.Event
	if ev := b.Start(); ev != nil {
		events = append(events, ev)
	}
	if b.currentThinkingIndex < 0 {
		b.currentThinkingIndex = len(b.partial.Content)
		b.partial.Content = append(b.partial.Content, &message.ThinkingContent{})
	}
	cur := b.partial.Content[b.currentThinkingIndex].(*message.ThinkingContent)
	cur.Thinking += delta
	if signature != "" {
		cur.ThinkingSignature = signature
	}
	if redacted {
		cur.Redacted = true
	}
	b.partial.Content[b.currentThinkingIndex] = cur
	if delta != "" {
		events = append(events, event.NewThinkingDelta(b.currentThinkingIndex, delta, signature, redacted, message.Clone(b.partial)))
	}
	return events
}

func (b *AssistantEventBuilder) ToolCallDelta(id, name, delta string) []event.Event {
	return b.toolCallDeltaInternal(-1, id, name, delta, "")
}

func (b *AssistantEventBuilder) ToolCallDeltaAt(index int, id, name, delta string) []event.Event {
	return b.toolCallDeltaInternal(index, id, name, delta, "")
}

func (b *AssistantEventBuilder) ToolCall(id, name string, args map[string]any, thoughtSignature ...string) []event.Event {
	raw := marshalJSONObject(args)
	if raw == "" {
		raw = "{}"
	}
	signature := ""
	if len(thoughtSignature) > 0 {
		signature = thoughtSignature[0]
	}
	return b.toolCallDeltaInternal(-1, id, name, raw, signature)
}

func (b *AssistantEventBuilder) toolCallDeltaInternal(index int, id, name, delta, thoughtSignature string) []event.Event {
	var events []event.Event
	if ev := b.Start(); ev != nil {
		events = append(events, ev)
	}

	state := b.lookupToolCallState(index, id, name)
	if state == nil {
		contentIndex := len(b.partial.Content)
		b.partial.Content = append(b.partial.Content, &message.ToolCallContent{
			ID:               id,
			Name:             name,
			ThoughtSignature: thoughtSignature,
		})
		state = &toolCallState{index: contentIndex}
		if index >= 0 {
			b.toolCallsByIndex[index] = state
		}
		if key := toolCallMapKey(id, name, contentIndex); key != "" {
			b.toolCalls[key] = state
		}
	}
	if !state.started {
		state.started = true
		events = append(events, event.NewToolCallDelta(state.index, id, name, "", message.Clone(b.partial)))
	}
	state.argsJSON += delta
	part := b.partial.Content[state.index].(*message.ToolCallContent)
	if part.ID == "" {
		part.ID = id
	}
	if part.Name == "" {
		part.Name = name
	}
	if thoughtSignature != "" {
		part.ThoughtSignature = thoughtSignature
	}
	if args, ok := decodeJSONObject(state.argsJSON); ok {
		part.Arguments = args
	}
	b.partial.Content[state.index] = part
	if index >= 0 {
		b.toolCallsByIndex[index] = state
	}
	if key := toolCallMapKey(part.ID, part.Name, state.index); key != "" {
		b.toolCalls[key] = state
	}
	if delta != "" {
		events = append(events, event.NewToolCallDelta(state.index, id, name, delta, message.Clone(b.partial)))
	}
	return events
}

func (b *AssistantEventBuilder) lookupToolCallState(index int, id, name string) *toolCallState {
	if index >= 0 {
		if state := b.toolCallsByIndex[index]; state != nil {
			return state
		}
	}
	if key := toolCallMapKey(id, name, index); key != "" {
		if state := b.toolCalls[key]; state != nil {
			return state
		}
	}
	return nil
}

func (b *AssistantEventBuilder) FinalizeToolCalls() []event.Event {
	var events []event.Event
	for _, state := range b.toolCalls {
		tc, ok := b.partial.Content[state.index].(*message.ToolCallContent)
		if !ok {
			continue
		}
		events = append(events, event.NewToolCallDelta(state.index, tc.ID, tc.Name, "", message.Clone(b.partial)))
	}
	return events
}

func (b *AssistantEventBuilder) FinalizeTextThinking() []event.Event {
	var events []event.Event
	if b.currentThinkingIndex >= 0 {
		events = append(events, event.NewThinkingDelta(b.currentThinkingIndex, "", contentStringAt(b.partial, b.currentThinkingIndex), false, message.Clone(b.partial)))
	}
	if b.currentTextIndex >= 0 {
		events = append(events, event.NewTextDelta(b.currentTextIndex, "", message.Clone(b.partial)))
	}
	return events
}

func (b *AssistantEventBuilder) Done(final *message.Message) event.Event {
	msg := message.Clone(b.partial)
	if final != nil {
		msg = message.Clone(final)
	}
	return event.NewMessageEnd(msg)
}

func (b *AssistantEventBuilder) Error(final *message.Message) event.Event {
	msg := message.Clone(b.partial)
	if final != nil {
		msg = message.Clone(final)
	}
	if msg != nil && msg.StopReason == "" {
		msg.StopReason = StopReasonError
	}
	return event.NewMessageEnd(msg)
}

type toolCallState struct {
	index        int
	started      bool
	argsJSON     string
	inputStarted bool
}

func contentStringAt(msg *message.Message, idx int) string {
	if msg == nil || idx >= len(msg.Content) {
		return ""
	}
	switch content := msg.Content[idx].(type) {
	case *message.TextContent:
		return content.Text
	case *message.ThinkingContent:
		return content.Thinking
	default:
		return ""
	}
}

func toolCallMapKey(id, name string, index int) string {
	if id != "" {
		return id
	}
	if name != "" {
		return fmt.Sprintf("%s:%d", name, index)
	}
	return ""
}

func decodeJSONObject(raw string) (map[string]any, bool) {
	var m map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &m) != nil {
		return nil, false
	}
	return m, true
}

func marshalJSONObject(args map[string]any) string {
	if args == nil {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(b)
}

