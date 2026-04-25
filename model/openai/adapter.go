// Package openai implements the ModelAdapter interface for OpenAI-compatible APIs.
// It uses the official openai-go SDK and supports any OpenAI-compatible endpoint
// (OpenAI, Azure, Groq, Cerebras, xAI, OpenRouter, Ollama, vLLM, etc.)
// by changing the BaseURL.
package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	modelcompat "github.com/nickqiaoo/tadk/model/internal/compat"
)

// Config holds configuration for the OpenAI adapter.
type Config struct {
	// APIKey is the API key for authentication.
	APIKey string
	// BaseURL is the API base URL. Defaults to OpenAI's API.
	// Change this for compatible providers (e.g., "https://api.groq.com/openai/v1").
	BaseURL string
	// Profile provides provider-specific behavior for OpenAI-compatible endpoints.
	// When set, it takes precedence over Compat.
	Profile ProviderProfile
	// Compat holds provider-specific quirks for OpenAI-compatible endpoints.
	// Deprecated: use Profile instead. Compat is kept for backward compatibility
	// and will be internally mapped to a ProviderProfile.
	Compat *CompatConfig
}

type adapter struct {
	client  openai.Client
	profile ProviderProfile
}

// New creates a new OpenAI ModelAdapter.
func New(cfg Config) model.ModelAdapter {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	baseURL := cfg.BaseURL
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	var profile ProviderProfile
	if cfg.Profile != nil {
		profile = cfg.Profile
	} else {
		profile = profileFromCompat(cfg.Compat)
	}

	return &adapter{
		client:  openai.NewClient(opts...),
		profile: profile,
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
			stream := a.client.Chat.Completions.NewStreaming(ctx, params, a.requestOptions(req)...)
			acc := openai.ChatCompletionAccumulator{}
			builder := model.NewAssistantEventBuilder()

			for stream.Next() {
				chunk := stream.Current()
				acc.AddChunk(chunk)

				for _, ev := range a.streamChunkEvents(builder, &chunk) {
					if !yield(ev, nil) {
						return
					}
				}
			}
			if err := stream.Err(); err != nil {
				finalErr = fmt.Errorf("openai: stream error: %w", err)
				yield(nil, finalErr)
				return
			}

			finalMsg = a.convertAccumulated(&acc, resolvedModel)
			if partial := builder.PartialMessage(); partial != nil && len(partial.Content) > 0 {
				finalMsg.Content = partial.Content
			}
			for _, ev := range builder.FinalizeTextThinking() {
				if !yield(ev, nil) {
					return
				}
			}
			for _, ev := range builder.FinalizeToolCalls() {
				if !yield(ev, nil) {
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

func (a *adapter) streamChunkEvents(builder *model.AssistantEventBuilder, chunk *openai.ChatCompletionChunk) []event.Event {
	if chunk == nil || len(chunk.Choices) == 0 {
		return nil
	}
	var events []event.Event
	delta := chunk.Choices[0].Delta
	if delta.Content != "" {
		events = append(events, builder.TextDelta(delta.Content)...)
	}
	if reasoning, field := openAIReasoningDelta(delta.RawJSON()); reasoning != "" {
		events = append(events, builder.ThinkingDelta(reasoning, field, false)...)
	}
	for _, tc := range delta.ToolCalls {
		events = append(events, builder.ToolCallDeltaAt(int(tc.Index), tc.ID, tc.Function.Name, tc.Function.Arguments)...)
	}
	return events
}

func (a *adapter) buildParams(req *model.Request) openai.ChatCompletionNewParams {
	cfg := a.mergedConfig(req)
	c := a.profile

	params := openai.ChatCompletionNewParams{
		Model:    a.resolvedModel(req),
		Messages: a.convertMessages(req),
	}

	if len(req.Tools) > 0 {
		params.Tools = a.convertTools(req.Tools)
	}

	c.ApplyParams(cfg, &params)
	return params
}

func (a *adapter) requestOptions(req *model.Request) []option.RequestOption {
	cfg := a.mergedConfig(req)
	c := a.profile
	return c.ApplyRequestOptions(cfg, nil)
}

// convertMessages converts canonical Messages to OpenAI chat messages.
func (a *adapter) convertMessages(req *model.Request) []openai.ChatCompletionMessageParamUnion {
	var msgs []openai.ChatCompletionMessageParamUnion
	c := a.profile

	// System prompt
	if req.SystemPrompt != "" {
		msgs = append(msgs, c.BuildSystemMessage(req.SystemPrompt))
	}

	transformedMessages := modelcompat.TransformMessagesForModel(
		req.Messages,
		a.resolvedModel(req),
		model.ProtocolOpenAICompletions,
		model.ProviderOpenAI,
		openAIToolCallIDNormalizer,
	)
	lastRole := message.Role("")

	for _, msg := range transformedMessages {
		if c.MaybeInjectAssistantAfterToolResult(lastRole, msg.Role) {
			msgs = append(msgs, openai.AssistantMessage("I have processed the tool results."))
		}

		switch msg.Role {
		case message.RoleUser:
			if content := openAIUserContent(msg, true); len(content) > 0 {
				if len(content) == 1 && content[0].OfText != nil {
					msgs = append(msgs, openai.UserMessage(content[0].OfText.Text))
				} else {
					msgs = append(msgs, openai.UserMessage(content))
				}
			}

		case message.RoleAssistant:
			assistantMsg := openai.ChatCompletionAssistantMessageParam{
				Role: "assistant",
			}

			text := c.FormatAssistantContent(msg)
			if text != "" {
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(text),
				}
			} else if c.RequiresAssistantAfterToolResult() {
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(""),
				}
			} else if extra := c.MaybeAddThinkingExtra(msg); extra != nil {
				assistantMsg.SetExtraFields(extra)
			}

			// Collect tool calls
			var reasoningDetails []any
			for _, tc := range msg.ToolCalls() {
				argsJSON, _ := json.Marshal(tc.Args)
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
				if tc.ThoughtSignature != "" {
					var detail any
					if json.Unmarshal([]byte(tc.ThoughtSignature), &detail) == nil {
						reasoningDetails = append(reasoningDetails, detail)
					}
				}
			}
			if len(reasoningDetails) > 0 {
				assistantMsg.SetExtraFields(map[string]any{"reasoning_details": reasoningDetails})
			}
			if text != "" || len(assistantMsg.ToolCalls) > 0 || len(reasoningDetails) > 0 {
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg})
			}

		case message.RoleToolResult:
			for _, tr := range msg.ToolResults() {
				toolMsg := c.BuildToolMessage(tr)
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfTool: &toolMsg})
			}
		}
		lastRole = msg.Role
	}

	return msgs
}

// convertTools converts canonical ToolDefinitions to OpenAI tool params.
func (a *adapter) convertTools(tools []model.ToolDefinition) []openai.ChatCompletionToolParam {
	var result []openai.ChatCompletionToolParam
	for _, t := range tools {
		result = append(result, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  openai.FunctionParameters(t.Parameters),
			},
		})
	}
	return result
}

// convertResponse converts an OpenAI completion to a canonical assistant message.
func (a *adapter) convertResponse(comp *openai.ChatCompletion, modelID string) *message.Message {
	if len(comp.Choices) == 0 {
		return &message.Message{
			Role:         message.RoleAssistant,
			Model:        modelID,
			StopReason:   message.StopReasonError,
			ErrorMessage: "no choices in response",
		}
	}

	choice := comp.Choices[0]
	msg := &message.Message{
		Role:       message.RoleAssistant,
		Protocol:   string(model.ProtocolOpenAICompletions),
		Provider:   string(model.ProviderOpenAI),
		Model:      modelID,
		ResponseID: comp.ID,
		StopReason: convertFinishReason(string(choice.FinishReason)),
	}

	if choice.Message.Content != "" {
		msg.Content = append(msg.Content, &message.TextContent{
			Text: choice.Message.Content,
		})
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		msg.Content = append(msg.Content, &message.ToolCallContent{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	if comp.Usage.TotalTokens > 0 {
		msg.Usage = &message.Usage{
			InputTokens:  int(comp.Usage.PromptTokens),
			OutputTokens: int(comp.Usage.CompletionTokens),
			TotalTokens:  int(comp.Usage.TotalTokens),
		}
	}

	return msg
}

// convertAccumulated converts the accumulated stream result to a final assistant message.
func (a *adapter) convertAccumulated(acc *openai.ChatCompletionAccumulator, modelID string) *message.Message {
	if len(acc.Choices) == 0 {
		return &message.Message{
			Role:         message.RoleAssistant,
			Protocol:     string(model.ProtocolOpenAICompletions),
			Provider:     string(model.ProviderOpenAI),
			Model:        modelID,
			StopReason:   message.StopReasonError,
			ErrorMessage: "no choices",
		}
	}

	choice := acc.Choices[0]
	msg := &message.Message{
		Role:       message.RoleAssistant,
		Protocol:   string(model.ProtocolOpenAICompletions),
		Provider:   string(model.ProviderOpenAI),
		Model:      modelID,
		ResponseID: acc.ID,
		StopReason: convertFinishReason(string(choice.FinishReason)),
	}

	if choice.Message.Content != "" {
		msg.Content = append(msg.Content, &message.TextContent{
			Text: choice.Message.Content,
		})
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		msg.Content = append(msg.Content, &message.ToolCallContent{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	if acc.Usage.TotalTokens > 0 {
		msg.Usage = &message.Usage{
			InputTokens:  int(acc.Usage.PromptTokens),
			OutputTokens: int(acc.Usage.CompletionTokens),
			TotalTokens:  int(acc.Usage.TotalTokens),
		}
	}

	return msg
}

func convertFinishReason(reason string) model.StopReason {
	switch reason {
	case "stop":
		return model.StopReasonStop
	case "length":
		return model.StopReasonLength
	case "tool_calls":
		return model.StopReasonToolUse
	case "content_filter":
		return model.StopReasonSafety
	default:
		return model.StopReasonUnknown
	}
}
