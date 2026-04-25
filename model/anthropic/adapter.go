// Package anthropic implements the ModelAdapter interface for the Anthropic Messages API.
// It uses the official anthropic-sdk-go SDK.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	modelcompat "github.com/nickqiaoo/tadk/model/internal/compat"
)

// Config holds configuration for the Anthropic adapter.
type Config struct {
	// APIKey is the Anthropic API key.
	APIKey string
	// BaseURL overrides the default Anthropic API endpoint.
	BaseURL string
}

type adapter struct {
	client anthropic.Client
}

// New creates a new Anthropic ModelAdapter.
func New(cfg Config) model.ModelAdapter {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	baseURL := cfg.BaseURL
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &adapter{
		client: anthropic.NewClient(opts...),
	}
}

func (a *adapter) Name() string {
	return ""
}

func (a *adapter) resolvedModel(req *model.Request) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	return ""
}

// mergedConfig returns the request's GenerateConfig. Treated as read-only by
// the adapter (see model.Request.Config); callers must not mutate the pointee.
func (a *adapter) mergedConfig(req *model.Request) *model.GenerateConfig {
	if req == nil {
		return nil
	}
	return req.Config
}

func (a *adapter) Generate(ctx context.Context, req *model.Request) (*message.Message, error) {
	return a.Stream(ctx, req).Result()
}

func (a *adapter) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	var finalMsg *message.Message
	var finalErr error
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			resolvedModel := a.resolvedModel(req)
			params := a.buildParams(req)
			stream := a.client.Messages.NewStreaming(ctx, params, a.requestOptions(req)...)
			var accumulated anthropic.Message
			var sawEvent bool
			builder := model.NewAssistantEventBuilder()
			if ev := builder.Start(); ev != nil {
				if !yield(ev, nil) {
					return
				}
			}

			for stream.Next() {
				ev := stream.Current()
				sawEvent = true
				if err := accumulated.Accumulate(ev); err != nil {
					finalErr = fmt.Errorf("anthropic: failed to accumulate stream event: %w", err)
					yield(nil, finalErr)
					return
				}
				for _, out := range a.streamEvents(builder, &ev, &accumulated) {
					if !yield(out, nil) {
						return
					}
				}
			}
			if err := stream.Err(); err != nil {
				finalErr = fmt.Errorf("anthropic: stream error: %w", err)
				yield(nil, finalErr)
				return
			}
			if !sawEvent {
				return
			}
			finalMsg = a.convertAccumulated(&accumulated, resolvedModel)
			for _, out := range builder.FinalizeTextThinking() {
				if !yield(out, nil) {
					return
				}
			}
			for _, out := range builder.FinalizeToolCalls() {
				if !yield(out, nil) {
					return
				}
			}
			yield(builder.Done(finalMsg), nil)
		},
		func() (*message.Message, error) {
			if finalMsg == nil {
				return nil, finalErr
			}
			return message.Clone(finalMsg), finalErr
		},
	)
}

func (a *adapter) streamEvents(
	builder *model.AssistantEventBuilder,
	ev *anthropic.MessageStreamEventUnion,
	acc *anthropic.Message,
) []event.Event {
	if ev == nil {
		return nil
	}
	switch ev.Type {
	case "content_block_start":
		block := ev.ContentBlock
		switch block.Type {
		case "tool_use", "server_tool_use":
			return builder.ToolCallDelta(block.ID, block.Name, "")
		case "redacted_thinking":
			return builder.ThinkingDelta("", block.Data, true)
		default:
			return nil
		}
	case "content_block_delta":
		delta := ev.Delta
		switch delta.Type {
		case "text_delta":
			return builder.TextDelta(delta.Text)
		case "thinking_delta":
			return builder.ThinkingDelta(delta.Thinking, "", false)
		case "input_json_delta":
			if acc == nil || len(acc.Content) == 0 {
				return nil
			}
			block := acc.Content[len(acc.Content)-1]
			if block.Type != "tool_use" && block.Type != "server_tool_use" {
				return nil
			}
			return builder.ToolCallDelta(block.ID, block.Name, delta.PartialJSON)
		default:
			return nil
		}
	default:
		return nil
	}
}

func (a *adapter) buildParams(req *model.Request) anthropic.MessageNewParams {
	const defaultMaxTokens = 4096
	maxTokens := int64(defaultMaxTokens)
	cfg := a.mergedConfig(req)
	if cfg != nil && cfg.MaxOutputTokens > 0 {
		maxTokens = int64(cfg.MaxOutputTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     a.resolvedModel(req),
		Messages:  a.convertMessages(req),
		MaxTokens: maxTokens,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = a.convertTools(req.Tools)
	}

	if cfg != nil {
		if cfg.Temperature != 0 {
			params.Temperature = param.NewOpt(cfg.Temperature)
		}
		if cfg.TopP != 0 {
			params.TopP = param.NewOpt(cfg.TopP)
		}
		if cfg.TopK > 0 {
			params.TopK = param.NewOpt(int64(cfg.TopK))
		}
		if len(cfg.StopSequences) > 0 {
			params.StopSequences = cfg.StopSequences
		}
		if md := anthropicMetadata(cfg); md != nil {
			params.Metadata = *md
		}
		if thinking := anthropicThinking(cfg); thinking != nil {
			params.Thinking = *thinking
		}
		if toolChoice := anthropicToolChoice(cfg); toolChoice != nil {
			params.ToolChoice = *toolChoice
		}
	}

	return params
}

func (a *adapter) requestOptions(req *model.Request) []option.RequestOption {
	cfg := a.mergedConfig(req)
	if cfg == nil {
		return nil
	}
	var opts []option.RequestOption
	for k, v := range cfg.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	if cfg.MaxRetryDelayMs > 0 {
		opts = append(opts, option.WithRequestTimeout(time.Duration(cfg.MaxRetryDelayMs)*time.Millisecond))
	}
	for k, v := range cfg.ProviderOptions {
		opts = append(opts, option.WithJSONSet(k, v))
	}
	return opts
}

// convertMessages converts canonical Messages to Anthropic MessageParams.
func (a *adapter) convertMessages(req *model.Request) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	msgs := modelcompat.TransformMessagesForModel(
		req.Messages,
		a.resolvedModel(req),
		model.ProtocolAnthropicMessages,
		model.ProviderAnthropic,
		anthropicToolCallIDNormalizer,
	)

	for _, msg := range msgs {
		switch msg.Role {
		case message.RoleUser:
			var blocks []anthropic.ContentBlockParamUnion
			for _, part := range msg.Content {
				switch c := part.(type) {
				case *message.TextContent:
					if strings.TrimSpace(c.Text) != "" {
						blocks = append(blocks, anthropic.NewTextBlock(c.Text))
					}
				case *message.ImageContent:
					if c.Data != "" {
						blocks = append(blocks, anthropic.NewImageBlockBase64(c.MimeType, c.Data))
					}
				}
			}
			for _, tr := range msg.ToolResults() {
				contentJSON, _ := json.Marshal(tr.Content)
				blocks = append(blocks, anthropic.NewToolResultBlock(tr.CallID, string(contentJSON), tr.IsError))
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Text()))
			}
			result = append(result, anthropic.NewUserMessage(blocks...))

		case message.RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			for _, part := range msg.Content {
				switch c := part.(type) {
				case *message.TextContent:
					if strings.TrimSpace(c.Text) != "" {
						blocks = append(blocks, anthropic.NewTextBlock(c.Text))
					}
				case *message.ToolCallContent:
					blocks = append(blocks, anthropic.NewToolUseBlock(c.ID, c.Arguments, c.Name))
				case *message.ThinkingContent:
					if c.Redacted && c.ThinkingSignature != "" {
						blocks = append(blocks, anthropic.NewRedactedThinkingBlock(c.ThinkingSignature))
					} else if c.ThinkingSignature != "" && strings.TrimSpace(c.Thinking) != "" {
						blocks = append(blocks, anthropic.NewThinkingBlock(c.ThinkingSignature, c.Thinking))
					} else if strings.TrimSpace(c.Thinking) != "" {
						blocks = append(blocks, anthropic.NewTextBlock(c.Thinking))
					}
				}
			}
			if len(blocks) > 0 {
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			}

		case message.RoleToolResult:
			// Anthropic puts tool results in user messages
			var blocks []anthropic.ContentBlockParamUnion
			for _, tr := range msg.ToolResults() {
				contentJSON, _ := json.Marshal(tr.Content)
				blocks = append(blocks, anthropic.NewToolResultBlock(tr.CallID, string(contentJSON), tr.IsError))
			}
			result = append(result, anthropic.NewUserMessage(blocks...))
		}
	}

	return result
}

var anthropicToolCallIDInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func anthropicToolCallIDNormalizer(id string, protocol model.MessageProtocol, provider model.Provider, modelID string, source *message.Message) string {
	if id == "" {
		return id
	}
	return truncateAnthropic(anthropicToolCallIDInvalidChars.ReplaceAllString(id, "_"), 64)
}

func truncateAnthropic(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// convertTools converts canonical ToolDefinitions to Anthropic ToolParams.
func (a *adapter) convertTools(tools []model.ToolDefinition) []anthropic.ToolUnionParam {
	var result []anthropic.ToolUnionParam
	for _, t := range tools {
		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: param.NewOpt(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: t.Parameters,
				},
			},
		})
	}
	return result
}

// convertResponse converts an Anthropic Message to a canonical assistant message.
func (a *adapter) convertResponse(resp *anthropic.Message, modelID string) *message.Message {
	msg := &message.Message{
		Role:       message.RoleAssistant,
		Protocol:   string(model.ProtocolAnthropicMessages),
		Provider:   string(model.ProviderAnthropic),
		Model:      modelID,
		ResponseID: resp.ID,
		StopReason: convertStopReason(resp.StopReason),
		Usage: &message.Usage{
			InputTokens:     int(resp.Usage.InputTokens),
			OutputTokens:    int(resp.Usage.OutputTokens),
			CacheReadTokens: int(resp.Usage.CacheReadInputTokens),
			TotalTokens:     int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		},
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			msg.Content = append(msg.Content, &message.TextContent{Text: block.Text})
		case "tool_use":
			var args map[string]any
			_ = json.Unmarshal(block.Input, &args)
			msg.Content = append(msg.Content, &message.ToolCallContent{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		case "thinking":
			msg.Content = append(msg.Content, &message.ThinkingContent{
				Thinking:          block.Thinking,
				ThinkingSignature: block.Signature,
			})
		case "redacted_thinking":
			msg.Content = append(msg.Content, &message.ThinkingContent{
				Redacted:          true,
				ThinkingSignature: block.Data,
			})
		}
	}

	return msg
}

func (a *adapter) convertAccumulated(resp *anthropic.Message, modelID string) *message.Message {
	return a.convertResponse(resp, modelID)
}

func convertStopReason(reason anthropic.StopReason) model.StopReason {
	switch reason {
	case "end_turn":
		return model.StopReasonStop
	case "max_tokens":
		return model.StopReasonLength
	case "tool_use":
		return model.StopReasonToolUse
	case "stop_sequence":
		return model.StopReasonStop
	case "refusal":
		return model.StopReasonSafety
	default:
		return model.StopReasonUnknown
	}
}

func anthropicMetadata(cfg *model.GenerateConfig) *anthropic.MetadataParam {
	if cfg == nil {
		return nil
	}
	var userID string
	if raw, ok := cfg.Metadata["user_id"]; ok {
		userID = fmt.Sprint(raw)
	}
	if userID == "" {
		userID = cfg.SessionID
	}
	if userID == "" {
		return nil
	}
	md := anthropic.MetadataParam{}
	md.UserID = param.NewOpt(userID)
	return &md
}

func anthropicThinking(cfg *model.GenerateConfig) *anthropic.ThinkingConfigParamUnion {
	if cfg == nil || cfg.ThinkingLevel == "" || cfg.ThinkingLevel == "off" {
		return nil
	}
	budget := anthropicThinkingBudget(cfg)
	thinking := anthropic.ThinkingConfigParamOfEnabled(int64(budget))
	return &thinking
}

func anthropicThinkingBudget(cfg *model.GenerateConfig) int {
	const minBudget = 1024
	if cfg == nil {
		return minBudget
	}
	if b := model.ConfiguredThinkingBudget(cfg.ThinkingBudgets, cfg.ThinkingLevel); b > 0 {
		if b < minBudget {
			return minBudget
		}
		return b
	}
	switch cfg.ThinkingLevel {
	case "minimal":
		return minBudget
	case "low":
		return 2048
	case "medium":
		return 4096
	case "high", "xhigh":
		return 8192
	default:
		return minBudget
	}
}

func anthropicToolChoice(cfg *model.GenerateConfig) *anthropic.ToolChoiceUnionParam {
	if cfg == nil || cfg.ToolChoice == nil {
		return nil
	}
	switch cfg.ToolChoice.Mode {
	case model.ToolChoiceNone:
		return &anthropic.ToolChoiceUnionParam{OfNone: &anthropic.ToolChoiceNoneParam{Type: "none"}}
	case model.ToolChoiceRequired:
		return &anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{Type: "any"}}
	case model.ToolChoiceTool:
		if cfg.ToolChoice.Name == "" {
			return nil
		}
		tc := anthropic.ToolChoiceParamOfTool(cfg.ToolChoice.Name)
		return &tc
	case model.ToolChoiceAuto:
		fallthrough
	default:
		return &anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{Type: "auto"}}
	}
}
