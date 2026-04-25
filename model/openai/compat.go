// Package openai implements the ModelAdapter interface for OpenAI-compatible APIs.
package openai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	oaishared "github.com/openai/openai-go/shared"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// CompatConfig holds OpenAI-compatible provider quirks.
// These are adapter-level constants, not per-request parameters.
type CompatConfig struct {
	SupportsStore                    bool
	SupportsDeveloperRole            bool
	SupportsReasoningEffort          bool
	ReasoningEffortMap               map[string]string
	SupportsUsageInStreaming         bool
	MaxTokensField                   string
	RequiresToolResultName           bool
	RequiresAssistantAfterToolResult bool
	RequiresThinkingAsText           bool
	ThinkingFormat                   string
	OpenRouterRouting                *OpenRouterRouting
	VercelGatewayRouting             *VercelGatewayRouting
	SupportsStrictMode               bool
}

// OpenRouterRouting holds routing preferences for OpenRouter.
type OpenRouterRouting struct {
	Only  []string
	Order []string
}

// VercelGatewayRouting holds routing preferences for Vercel AI Gateway.
type VercelGatewayRouting struct {
	Only  []string
	Order []string
}

// compat wraps OpenAI-compatible provider quirks.
type compat struct {
	*CompatConfig
}

// defaultCompat returns the default compat wrapper.
func defaultCompat() compat {
	return compat{&CompatConfig{
		SupportsStore:           true,
		SupportsDeveloperRole:   true,
		SupportsReasoningEffort: true,
		MaxTokensField:          "max_completion_tokens",
		ThinkingFormat:          "openai",
	}}
}

// MaxTokensField implements ProviderProfile.
func (c compat) maxTokensField() string {
	if c.CompatConfig != nil && c.MaxTokensField != "" {
		return c.MaxTokensField
	}
	return "max_completion_tokens"
}

// SupportsStore implements ProviderProfile.
func (c compat) supportsStore() bool {
	return c.SupportsStore
}

// SupportsReasoningEffort implements ProviderProfile.
func (c compat) supportsReasoningEffort() bool {
	return c.SupportsReasoningEffort
}

// SupportsDeveloperRole implements ProviderProfile.
func (c compat) supportsDeveloperRole() bool {
	return c.SupportsDeveloperRole
}

// RequiresThinkingAsText implements ProviderProfile.
func (c compat) requiresThinkingAsText() bool {
	return c.RequiresThinkingAsText
}

func (c compat) requiresToolResultName() bool {
	return c.RequiresToolResultName
}

func (c compat) requiresAssistantAfterToolResult() bool {
	return c.RequiresAssistantAfterToolResult
}

// MaybeInjectAssistantAfterToolResult implements ProviderProfile.
func (c compat) maybeInjectAssistantAfterToolResult(lastRole message.Role, nextRole message.Role) bool {
	return c.RequiresAssistantAfterToolResult && lastRole == message.RoleToolResult && nextRole == message.RoleUser
}

func (c compat) thinkingFormat() string {
	if c.CompatConfig != nil && c.ThinkingFormat != "" {
		return c.ThinkingFormat
	}
	return "openai"
}

func (c compat) openRouterRouting() *OpenRouterRouting {
	if c.CompatConfig != nil {
		return c.OpenRouterRouting
	}
	return nil
}

func (c compat) vercelGatewayRouting() *VercelGatewayRouting {
	if c.CompatConfig != nil {
		return c.VercelGatewayRouting
	}
	return nil
}

// ReasoningEffort implements ProviderProfile.
func (c compat) reasoningEffort(cfg *model.GenerateConfig) (string, bool) {
	if cfg == nil || cfg.ThinkingLevel == "" || cfg.ThinkingLevel == "off" {
		return "", false
	}
	level := cfg.ThinkingLevel
	if c.CompatConfig != nil && c.ReasoningEffortMap != nil {
		if mapped, ok := c.ReasoningEffortMap[level]; ok && mapped != "" {
			return mapped, true
		}
	}
	switch level {
	case "minimal", "low":
		return "low", true
	case "medium":
		return "medium", true
	case "high", "xhigh":
		return "high", true
	default:
		return "", false
	}
}

func thinkingEnabled(cfg *model.GenerateConfig) bool {
	return cfg != nil && cfg.ThinkingLevel != "" && cfg.ThinkingLevel != "off"
}

// ApplyParams implements ProviderProfile.
func (c compat) applyParams(cfg *model.GenerateConfig, params *openai.ChatCompletionNewParams) {
	if cfg == nil {
		return
	}
	if cfg.Temperature != 0 {
		params.Temperature = openai.Float(cfg.Temperature)
	}
	if cfg.TopP != 0 {
		params.TopP = openai.Float(cfg.TopP)
	}
	if cfg.MaxOutputTokens > 0 {
		if strings.EqualFold(c.maxTokensField(), "max_tokens") {
			params.MaxTokens = openai.Int(int64(cfg.MaxOutputTokens))
		} else {
			params.MaxCompletionTokens = openai.Int(int64(cfg.MaxOutputTokens))
		}
	}
	if len(cfg.StopSequences) > 0 {
		params.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: cfg.StopSequences}
	}
	if cfg.Seed != 0 {
		params.Seed = openai.Int(int64(cfg.Seed))
	}
	if cfg.Store && c.supportsStore() {
		params.Store = openai.Bool(true)
	}
	if cfg.ParallelToolCalls {
		params.ParallelToolCalls = openai.Bool(true)
	}
	if cfg.PromptCacheKey != "" {
		params.PromptCacheKey = openai.String(cfg.PromptCacheKey)
	}
	if cfg.SafetyIdentifier != "" {
		params.SafetyIdentifier = openai.String(cfg.SafetyIdentifier)
	}
	if cfg.ServiceTier != "" {
		params.ServiceTier = openai.ChatCompletionNewParamsServiceTier(cfg.ServiceTier)
	}
	if md := model.StringMap(cfg.Metadata); len(md) > 0 {
		params.Metadata = oaishared.Metadata(md)
	}
	if effort, ok := c.reasoningEffort(cfg); ok && c.supportsReasoningEffort() {
		params.ReasoningEffort = oaishared.ReasoningEffort(effort)
	}
	if cfg.ResponseSchema != nil {
		// Structured outputs are applied via request options instead.
	} else if cfg.ResponseMIMEType == "application/json" {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &oaishared.ResponseFormatJSONObjectParam{},
		}
	}
}

// ApplyRequestOptions implements ProviderProfile.
func (c compat) applyRequestOptions(cfg *model.GenerateConfig, opts []option.RequestOption) []option.RequestOption {
	if cfg == nil {
		return opts
	}
	for k, v := range cfg.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	if cfg.MaxRetryDelayMs > 0 {
		opts = append(opts, option.WithRequestTimeout(time.Duration(cfg.MaxRetryDelayMs)*time.Millisecond))
	}
	if cfg.ResponseSchema != nil {
		opts = append(opts, option.WithJSONSet("response_format", map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"schema": cfg.ResponseSchema,
			},
		}))
	}
	switch c.thinkingFormat() {
	case "openrouter":
		if effort, ok := c.reasoningEffort(cfg); ok {
			opts = append(opts, option.WithJSONSet("reasoning", map[string]any{"effort": effort}))
		}
	case "zai", "qwen":
		opts = append(opts, option.WithJSONSet("enable_thinking", thinkingEnabled(cfg)))
	case "qwen-chat-template":
		opts = append(opts, option.WithJSONSet("chat_template_kwargs", map[string]any{"enable_thinking": thinkingEnabled(cfg)}))
	}
	if routing := c.openRouterRouting(); routing != nil {
		opts = append(opts, option.WithJSONSet("provider", routing))
	}
	if routing := c.vercelGatewayRouting(); routing != nil {
		if len(routing.Only) > 0 {
			opts = append(opts, option.WithHeader("x-vercel-ai-gateway-provider", strings.Join(routing.Only, ",")))
		}
		if len(routing.Order) > 0 {
			opts = append(opts, option.WithHeader("x-vercel-ai-gateway-fallback-order", strings.Join(routing.Order, ",")))
		}
	}
	for k, v := range cfg.ProviderOptions {
		opts = append(opts, option.WithJSONSet(k, v))
	}
	return opts
}

// BuildSystemMessage implements ProviderProfile.
func (c compat) buildSystemMessage(prompt string) openai.ChatCompletionMessageParamUnion {
	if c.supportsDeveloperRole() {
		return openai.DeveloperMessage(prompt)
	}
	return openai.SystemMessage(prompt)
}

// FormatAssistantContent implements ProviderProfile.
func (c compat) formatAssistantContent(msg *message.Message) string {
	text := msg.Text()
	if c.requiresThinkingAsText() {
		thinking := strings.TrimSpace(joinThinking(msg))
		if thinking != "" {
			if text != "" {
				text = thinking + "\n\n" + text
			} else {
				text = thinking
			}
		}
	}
	return text
}

// MaybeAddThinkingExtra implements ProviderProfile.
func (c compat) maybeAddThinkingExtra(msg *message.Message) map[string]any {
	if c.requiresThinkingAsText() {
		return nil
	}
	field, text := openAIThinkingField(msg)
	if field != "" && text != "" {
		return map[string]any{field: text}
	}
	return nil
}

// BuildToolMessage implements ProviderProfile.
func (c compat) buildToolMessage(tr message.ToolResult) openai.ChatCompletionToolMessageParam {
	content := toolResultText(tr)
	out := openai.ChatCompletionToolMessageParam{
		Content: openai.ChatCompletionToolMessageParamContentUnion{
			OfString: param.NewOpt(content),
		},
		ToolCallID: tr.CallID,
	}
	if c.requiresToolResultName() && tr.Name != "" {
		out.SetExtraFields(map[string]any{"name": tr.Name})
	}
	return out
}

var openAIToolCallIDInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func openAIToolCallIDNormalizer(id string, protocol model.MessageProtocol, provider model.Provider, modelID string, source *message.Message) string {
	if id == "" {
		return id
	}
	if strings.Contains(id, "|") {
		callID := strings.SplitN(id, "|", 2)[0]
		return truncate(openAIToolCallIDInvalidChars.ReplaceAllString(callID, "_"), 40)
	}
	if provider == model.ProviderOpenAI {
		return truncate(id, 40)
	}
	return id
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func openAIReasoningDelta(raw string) (string, string) {
	if raw == "" {
		return "", ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return "", ""
	}
	for _, field := range []string{"reasoning_content", "reasoning", "reasoning_text"} {
		if v, ok := obj[field].(string); ok && v != "" {
			return v, field
		}
	}
	return "", ""
}

func openAIThinkingField(msg *message.Message) (string, string) {
	if msg == nil {
		return "", ""
	}
	var field string
	var chunks []string
	for _, part := range msg.ThinkingContents() {
		if tc, ok := part.(*message.ThinkingContent); ok {
			if field == "" && tc.ThinkingSignature != "" {
				field = tc.ThinkingSignature
			}
			if strings.TrimSpace(tc.Thinking) != "" {
				chunks = append(chunks, tc.Thinking)
			}
		}
	}
	if field == "" || len(chunks) == 0 {
		return "", ""
	}
	return field, strings.Join(chunks, "\n")
}

func joinThinking(msg *message.Message) string {
	if msg == nil {
		return ""
	}
	var chunks []string
	for _, part := range msg.ThinkingContents() {
		if tc, ok := part.(*message.ThinkingContent); ok && strings.TrimSpace(tc.Thinking) != "" {
			chunks = append(chunks, tc.Thinking)
		}
	}
	return strings.Join(chunks, "\n\n")
}

func toolResultText(tr message.ToolResult) string {
	if s, ok := tr.Content.(string); ok {
		if s != "" {
			return s
		}
	}
	b, err := json.Marshal(tr.Content)
	if err == nil && string(b) != "null" {
		return string(b)
	}
	return ""
}

func openAIUserContent(msg *message.Message, includeImages bool) []openai.ChatCompletionContentPartUnionParam {
	if msg == nil {
		return nil
	}
	var parts []openai.ChatCompletionContentPartUnionParam
	for _, content := range msg.Content {
		switch c := content.(type) {
		case *message.TextContent:
			if strings.TrimSpace(c.Text) != "" {
				parts = append(parts, openai.TextContentPart(c.Text))
			}
		case *message.ImageContent:
			if includeImages {
				url := c.URL
				if url == "" && c.Data != "" {
					url = fmt.Sprintf("data:%s;base64,%s", c.MimeType, c.Data)
				}
				if url != "" {
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
						URL: url,
					}))
				}
			}
		}
	}
	if len(parts) == 0 && strings.TrimSpace(msg.Text()) != "" {
		parts = append(parts, openai.TextContentPart(msg.Text()))
	}
	return parts
}
