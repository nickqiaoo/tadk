// Package message defines the canonical message types used throughout the framework.
// The public shape follows the PI architecture:
// - user messages carry user content
// - assistant messages carry assistant content plus provider metadata
// - tool-result messages carry tool result payloads plus optional content
package message

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "toolResult"
)

type Usage struct {
	InputTokens      int `json:"input,omitempty"`
	OutputTokens     int `json:"output,omitempty"`
	CacheReadTokens  int `json:"cacheRead,omitempty"`
	CacheWriteTokens int `json:"cacheWrite,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "toolUse"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
	StopReasonSafety  StopReason = "safety"
	StopReasonUnknown StopReason = "unknown"
)

type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "toolCall"
)

// Content is a content part carried by a Message.
type Content interface {
	contentType() ContentType
}

// TextContent carries plain text.
type TextContent struct {
	Text          string `json:"text,omitempty"`
	TextSignature string `json:"textSignature,omitempty"`
}

func (*TextContent) contentType() ContentType { return ContentTypeText }

// MarshalJSON writes the canonical tagged-union shape.
func (c *TextContent) MarshalJSON() ([]byte, error) {
	type alias TextContent
	return json.Marshal(&struct {
		Type ContentType `json:"type"`
		*alias
	}{Type: ContentTypeText, alias: (*alias)(c)})
}

// ThinkingContent carries model reasoning text.
type ThinkingContent struct {
	Thinking          string `json:"thinking,omitempty"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

func (*ThinkingContent) contentType() ContentType { return ContentTypeThinking }

// MarshalJSON writes the canonical tagged-union shape.
func (c *ThinkingContent) MarshalJSON() ([]byte, error) {
	type alias ThinkingContent
	return json.Marshal(&struct {
		Type ContentType `json:"type"`
		*alias
	}{Type: ContentTypeThinking, alias: (*alias)(c)})
}

// ImageContent carries inline image data or a URL.
type ImageContent struct {
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URL      string `json:"url,omitempty"`
}

func (*ImageContent) contentType() ContentType { return ContentTypeImage }

// MarshalJSON writes the canonical tagged-union shape.
func (c *ImageContent) MarshalJSON() ([]byte, error) {
	type alias ImageContent
	return json.Marshal(&struct {
		Type ContentType `json:"type"`
		*alias
	}{Type: ContentTypeImage, alias: (*alias)(c)})
}

// ToolCallContent carries a request from the model to invoke a tool.
type ToolCallContent struct {
	ID               string         `json:"id,omitempty"`
	Name             string         `json:"name,omitempty"`
	Arguments        map[string]any `json:"arguments,omitempty"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

func (*ToolCallContent) contentType() ContentType { return ContentTypeToolCall }

// MarshalJSON writes the canonical tagged-union shape.
func (c *ToolCallContent) MarshalJSON() ([]byte, error) {
	type alias ToolCallContent
	return json.Marshal(&struct {
		Type ContentType `json:"type"`
		*alias
	}{Type: ContentTypeToolCall, alias: (*alias)(c)})
}

type ToolCall struct {
	ID               string         `json:"id,omitempty"`
	Name             string         `json:"name,omitempty"`
	Args             map[string]any `json:"args,omitempty"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

type ToolResult struct {
	CallID  string `json:"callId,omitempty"`
	Name    string `json:"name,omitempty"`
	Content any    `json:"content,omitempty"`
	IsError bool   `json:"isError,omitempty"`
}

// Message is the canonical message record used across the framework.
type Message struct {
	Role    Role      `json:"-"`
	Content []Content `json:"-"`

	ToolResultsData []ToolResult `json:"-"`

	Protocol     string     `json:"-"`
	Provider     string     `json:"-"`
	Model        string     `json:"-"`
	ResponseID   string     `json:"-"`
	Usage        *Usage     `json:"-"`
	StopReason   StopReason `json:"-"`
	ErrorCode    string     `json:"-"`
	ErrorMessage string     `json:"-"`
	Timestamp    int64      `json:"-"`
}

type userMessageWire struct {
	Role      Role  `json:"role"`
	Content   any   `json:"content"`
	Timestamp int64 `json:"timestamp,omitempty"`
}

type assistantMessageWire struct {
	Role         Role              `json:"role"`
	Content      []json.RawMessage `json:"content"`
	Protocol     string            `json:"protocol,omitempty"`
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	ResponseID   string            `json:"responseId,omitempty"`
	Usage        *Usage            `json:"usage,omitempty"`
	StopReason   StopReason        `json:"stopReason,omitempty"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
	Timestamp    int64             `json:"timestamp,omitempty"`
}

type toolResultWire struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Details    any    `json:"details,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
}

type toolResultMessageWire struct {
	Role        Role              `json:"role"`
	Content     []json.RawMessage `json:"content,omitempty"`
	ToolResults []toolResultWire  `json:"toolResults,omitempty"`
	Timestamp   int64             `json:"timestamp,omitempty"`

	// Backward-compatible singular fields for older persisted sessions.
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Details    any    `json:"details,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	switch m.Role {
	case RoleUser:
		return marshalUserLikeMessage(&m)
	case RoleAssistant:
		return marshalAssistantMessage(&m)
	case RoleToolResult:
		return marshalToolResultMessage(&m)
	default:
		return nil, fmt.Errorf("message: unsupported role %q", m.Role)
	}
}

func (m *Message) UnmarshalJSON(data []byte) error {
	var header struct {
		Role    Role            `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}

	switch header.Role {
	case RoleUser:
		var wire userMessageWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		m.Role = wire.Role
		m.Timestamp = wire.Timestamp
		content, err := decodeUserContent(wire.Content)
		if err != nil {
			return err
		}
		m.Content = content
		m.ToolResultsData = nil
		return nil
	case RoleAssistant:
		var wire assistantMessageWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		m.Role = wire.Role
		m.Protocol = wire.Protocol
		m.Provider = wire.Provider
		m.Model = wire.Model
		m.ResponseID = wire.ResponseID
		m.Usage = wire.Usage
		m.StopReason = wire.StopReason
		m.ErrorMessage = wire.ErrorMessage
		m.Timestamp = wire.Timestamp
		content, err := decodeContentArray(wire.Content)
		if err != nil {
			return err
		}
		m.Content = content
		m.ToolResultsData = nil
		return nil
	case RoleToolResult:
		var wire toolResultMessageWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		m.Role = wire.Role
		m.Timestamp = wire.Timestamp
		content, err := decodeContentArray(wire.Content)
		if err != nil {
			return err
		}
		m.Content = content
		if len(wire.ToolResults) > 0 {
			m.ToolResultsData = make([]ToolResult, 0, len(wire.ToolResults))
			for _, tr := range wire.ToolResults {
				m.ToolResultsData = append(m.ToolResultsData, ToolResult{
					CallID:  tr.ToolCallID,
					Name:    tr.ToolName,
					Content: tr.Details,
					IsError: tr.IsError,
				})
			}
		} else if wire.ToolCallID != "" || wire.ToolName != "" || wire.Details != nil {
			m.ToolResultsData = []ToolResult{{
				CallID:  wire.ToolCallID,
				Name:    wire.ToolName,
				Content: wire.Details,
				IsError: wire.IsError,
			}}
		}
		return nil
	default:
		return fmt.Errorf("message: unsupported role %q", header.Role)
	}
}

func NewUserMessage(text string) *Message {
	return &Message{
		Role:    RoleUser,
		Content: []Content{&TextContent{Text: text}},
	}
}

func NewAssistantMessage(text string) *Message {
	return &Message{
		Role:    RoleAssistant,
		Content: []Content{&TextContent{Text: text}},
	}
}

func NewToolResultMessage(callID string, content any, isError bool) *Message {
	return &Message{
		Role:    RoleToolResult,
		Content: toolResultContentFromAny(content),
		ToolResultsData: []ToolResult{{
			CallID:  callID,
			Content: content,
			IsError: isError,
		}},
	}
}

func NewToolResultMessages(results []ToolResult, content []Content) *Message {
	return &Message{
		Role:            RoleToolResult,
		Content:         cloneContents(content),
		ToolResultsData: cloneToolResults(results),
	}
}

func NewMessage(role Role, content []Content) *Message {
	return &Message{
		Role:    role,
		Content: cloneContents(content),
	}
}

// EnsureToolCallIDs normalizes assistant tool-call content so every tool call has an ID.
func EnsureToolCallIDs(msg *Message) {
	if msg == nil {
		return
	}
	for _, c := range msg.Content {
		tc, ok := c.(*ToolCallContent)
		if !ok || tc.ID != "" {
			continue
		}
		tc.ID = uuid.NewString()
	}
}

func (m *Message) Text() string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range m.Content {
		if tc, ok := c.(*TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if m.Role == RoleToolResult && b.Len() == 0 {
		for _, tr := range m.ToolResults() {
			if text, ok := tr.Content.(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

// TextContents returns text parts as a read-only view of the underlying content slice.
// Callers must not mutate returned items.
func (m *Message) TextContents() []Content {
	if m == nil {
		return nil
	}
	var out []Content
	for _, c := range m.Content {
		if _, ok := c.(*TextContent); ok {
			out = append(out, c)
		}
	}
	return out
}

// ThinkingContents returns thinking parts as a read-only view.
// Callers must not mutate returned items.
func (m *Message) ThinkingContents() []Content {
	if m == nil {
		return nil
	}
	var out []Content
	for _, c := range m.Content {
		if _, ok := c.(*ThinkingContent); ok {
			out = append(out, c)
		}
	}
	return out
}

// ToolCalls returns tool calls as a read-only view; Args maps share storage with the underlying content.
// Callers must not mutate returned items.
func (m *Message) ToolCalls() []ToolCall {
	if m == nil {
		return nil
	}
	var calls []ToolCall
	for _, c := range m.Content {
		tc, ok := c.(*ToolCallContent)
		if !ok {
			continue
		}
		calls = append(calls, ToolCall{
			ID:               tc.ID,
			Name:             tc.Name,
			Args:             tc.Arguments,
			ThoughtSignature: tc.ThoughtSignature,
		})
	}
	return calls
}

func (m *Message) HasToolCalls() bool {
	if m == nil {
		return false
	}
	for _, c := range m.Content {
		if _, ok := c.(*ToolCallContent); ok {
			return true
		}
	}
	return false
}

// ToolResults returns stored tool results as a read-only view.
// Callers must not mutate returned items.
func (m *Message) ToolResults() []ToolResult {
	if m == nil {
		return nil
	}
	return m.ToolResultsData
}

func Clone(src *Message) *Message {
	if src == nil {
		return nil
	}
	dst := &Message{
		Role:            src.Role,
		Content:         cloneContents(src.Content),
		ToolResultsData: cloneToolResults(src.ToolResultsData),
		Protocol:        src.Protocol,
		Provider:        src.Provider,
		Model:           src.Model,
		ResponseID:      src.ResponseID,
		StopReason:      src.StopReason,
		ErrorCode:       src.ErrorCode,
		ErrorMessage:    src.ErrorMessage,
		Timestamp:       src.Timestamp,
	}
	if src.Usage != nil {
		usage := *src.Usage
		dst.Usage = &usage
	}
	return dst
}

func toolResultContentFromAny(value any) []Content {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return []Content{&TextContent{Text: v}}
	case Content:
		return []Content{cloneContent(v)}
	case []Content:
		return cloneContents(v)
	default:
		return nil
	}
}

func marshalUserLikeMessage(msg *Message) ([]byte, error) {
	content := userContentForMarshal(msg.Content)
	return json.Marshal(userMessageWire{
		Role:      msg.Role,
		Content:   content,
		Timestamp: msg.Timestamp,
	})
}

func marshalAssistantMessage(msg *Message) ([]byte, error) {
	content, err := marshalContentArray(msg.Content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(assistantMessageWire{
		Role:         msg.Role,
		Content:      content,
		Protocol:     msg.Protocol,
		Provider:     msg.Provider,
		Model:        msg.Model,
		ResponseID:   msg.ResponseID,
		Usage:        msg.Usage,
		StopReason:   msg.StopReason,
		ErrorMessage: msg.ErrorMessage,
		Timestamp:    msg.Timestamp,
	})
}

func marshalToolResultMessage(msg *Message) ([]byte, error) {
	content, err := marshalContentArray(msg.Content)
	if err != nil {
		return nil, err
	}
	results := make([]toolResultWire, 0, len(msg.ToolResultsData))
	for _, tr := range msg.ToolResultsData {
		results = append(results, toolResultWire{
			ToolCallID: tr.CallID,
			ToolName:   tr.Name,
			Details:    tr.Content,
			IsError:    tr.IsError,
		})
	}
	return json.Marshal(toolResultMessageWire{
		Role:        msg.Role,
		Content:     content,
		ToolResults: results,
		Timestamp:   msg.Timestamp,
	})
}

func userContentForMarshal(content []Content) any {
	if len(content) == 1 {
		if tc, ok := content[0].(*TextContent); ok && tc.TextSignature == "" {
			return tc.Text
		}
	}
	raw, _ := marshalContentArray(content)
	return raw
}

func marshalContentArray(content []Content) ([]json.RawMessage, error) {
	if len(content) == 0 {
		return nil, nil
	}
	out := make([]json.RawMessage, 0, len(content))
	for _, c := range content {
		raw, err := marshalContent(c)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func marshalContent(content Content) (json.RawMessage, error) {
	return json.Marshal(content)
}

func decodeUserContent(raw any) ([]Content, error) {
	switch v := raw.(type) {
	case string:
		return []Content{&TextContent{Text: v}}, nil
	case []any:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, err
		}
		return decodeContentArray(arr)
	default:
		return nil, nil
	}
}

func decodeContentArray(raw []json.RawMessage) ([]Content, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]Content, 0, len(raw))
	for _, item := range raw {
		content, err := unmarshalContent(item)
		if err != nil {
			return nil, err
		}
		out = append(out, content)
	}
	return out, nil
}

func unmarshalContent(raw json.RawMessage) (Content, error) {
	var wrapper struct {
		Type ContentType `json:"type"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	switch wrapper.Type {
	case ContentTypeText:
		var c TextContent
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		return &c, nil
	case ContentTypeThinking:
		var c ThinkingContent
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		return &c, nil
	case ContentTypeImage:
		var c ImageContent
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		return &c, nil
	case ContentTypeToolCall:
		var c ToolCallContent
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		c.Arguments = cloneMap(c.Arguments)
		return &c, nil
	default:
		return nil, fmt.Errorf("message: unknown content type %q", wrapper.Type)
	}
}

func cloneContents(src []Content) []Content {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Content, 0, len(src))
	for _, c := range src {
		dst = append(dst, cloneContent(c))
	}
	return dst
}

func cloneContent(src Content) Content {
	switch c := src.(type) {
	case *TextContent:
		cp := *c
		return &cp
	case *ThinkingContent:
		cp := *c
		return &cp
	case *ImageContent:
		cp := *c
		return &cp
	case *ToolCallContent:
		cp := *c
		cp.Arguments = cloneMap(c.Arguments)
		return &cp
	default:
		return src
	}
}

func cloneToolResults(src []ToolResult) []ToolResult {
	if len(src) == 0 {
		return nil
	}
	dst := make([]ToolResult, 0, len(src))
	for _, tr := range src {
		dst = append(dst, ToolResult{
			CallID:  tr.CallID,
			Name:    tr.Name,
			Content: tr.Content,
			IsError: tr.IsError,
		})
	}
	return dst
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
