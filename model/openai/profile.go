package openai

import (
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// ProviderProfile abstracts provider-specific behavior for OpenAI-compatible
// endpoints. Implementations handle quirks of individual providers (OpenAI,
// Groq, OpenRouter, Azure, Ollama, etc.) without polluting a central
// CompatConfig struct.
//
// ProviderProfile is intentionally defined inside the openai package because
// these behaviors are specific to the OpenAI protocol adapter.
type ProviderProfile interface {
	// Name returns a diagnostic label for the profile.
	Name() string

	// ApplyParams mutates the OpenAI request params according to the provider's
	// supported generation parameters.
	ApplyParams(cfg *model.GenerateConfig, params *openai.ChatCompletionNewParams)

	// ApplyRequestOptions returns provider-specific request options (headers,
	// JSON overrides, timeouts).
	ApplyRequestOptions(cfg *model.GenerateConfig, opts []option.RequestOption) []option.RequestOption

	// BuildSystemMessage returns the appropriate system/developer message param.
	BuildSystemMessage(prompt string) openai.ChatCompletionMessageParamUnion

	// FormatAssistantContent post-processes assistant content before sending.
	FormatAssistantContent(msg *message.Message) string

	// MaybeInjectAssistantAfterToolResult reports whether an assistant message
	// must be injected between a tool result and a user message.
	MaybeInjectAssistantAfterToolResult(lastRole message.Role, nextRole message.Role) bool

	// MaybeAddThinkingExtra returns provider-specific extra fields for thinking.
	MaybeAddThinkingExtra(msg *message.Message) map[string]any

	// RequiresThinkingAsText reports whether thinking must be collapsed into text.
	RequiresThinkingAsText() bool

	// RequiresAssistantAfterToolResult reports whether an assistant message
	// must follow a tool result.
	RequiresAssistantAfterToolResult() bool

	// BuildToolMessage constructs a tool message param with provider quirks.
	BuildToolMessage(tr message.ToolResult) openai.ChatCompletionToolMessageParam

	// MaxTokensField returns the field name to use for max tokens.
	MaxTokensField() string

	// SupportsStrictMode reports whether structured output strict mode is supported.
	SupportsStrictMode() bool

	// ReasoningEffort returns the reasoning effort value and whether it is supported.
	ReasoningEffort(cfg *model.GenerateConfig) (string, bool)

	// SupportsReasoningEffort reports whether reasoning effort is supported.
	SupportsReasoningEffort() bool

	// SupportsStore reports whether the store parameter is supported.
	SupportsStore() bool

	// SupportsDeveloperRole reports whether the developer role is supported.
	SupportsDeveloperRole() bool
}

// compatProfile wraps the unexported compat type so that it can implement
// ProviderProfile without method/field name collisions on the embedded
// CompatConfig.
type compatProfile struct {
	c compat
}

func (p compatProfile) Name() string { return "" }
func (p compatProfile) ApplyParams(cfg *model.GenerateConfig, params *openai.ChatCompletionNewParams) {
	p.c.applyParams(cfg, params)
}
func (p compatProfile) ApplyRequestOptions(cfg *model.GenerateConfig, opts []option.RequestOption) []option.RequestOption {
	return p.c.applyRequestOptions(cfg, opts)
}
func (p compatProfile) BuildSystemMessage(prompt string) openai.ChatCompletionMessageParamUnion {
	return p.c.buildSystemMessage(prompt)
}
func (p compatProfile) FormatAssistantContent(msg *message.Message) string {
	return p.c.formatAssistantContent(msg)
}
func (p compatProfile) MaybeInjectAssistantAfterToolResult(lastRole message.Role, nextRole message.Role) bool {
	return p.c.maybeInjectAssistantAfterToolResult(lastRole, nextRole)
}
func (p compatProfile) MaybeAddThinkingExtra(msg *message.Message) map[string]any {
	return p.c.maybeAddThinkingExtra(msg)
}
func (p compatProfile) RequiresThinkingAsText() bool { return p.c.requiresThinkingAsText() }
func (p compatProfile) RequiresAssistantAfterToolResult() bool {
	return p.c.requiresAssistantAfterToolResult()
}
func (p compatProfile) BuildToolMessage(tr message.ToolResult) openai.ChatCompletionToolMessageParam {
	return p.c.buildToolMessage(tr)
}
func (p compatProfile) MaxTokensField() string { return p.c.maxTokensField() }
func (p compatProfile) SupportsStrictMode() bool { return p.c.SupportsStrictMode }
func (p compatProfile) ReasoningEffort(cfg *model.GenerateConfig) (string, bool) {
	return p.c.reasoningEffort(cfg)
}
func (p compatProfile) SupportsReasoningEffort() bool { return p.c.supportsReasoningEffort() }
func (p compatProfile) SupportsStore() bool { return p.c.supportsStore() }
func (p compatProfile) SupportsDeveloperRole() bool { return p.c.supportsDeveloperRole() }

// defaultProfile returns the default provider profile for standard OpenAI.
func defaultProfile() ProviderProfile {
	return compatProfile{c: defaultCompat()}
}

// profileFromCompat creates a ProviderProfile from a legacy CompatConfig.
// This exists for backward compatibility while users migrate to ProviderProfile.
func profileFromCompat(cfg *CompatConfig) ProviderProfile {
	c := compat{cfg}
	if c.CompatConfig == nil {
		c.CompatConfig = defaultCompat().CompatConfig
	}
	return compatProfile{c: c}
}
