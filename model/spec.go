package model

type MessageProtocol string
type Provider string
type InputType string

const (
	ProtocolGeminiGenerateContent MessageProtocol = "gemini-generate-content"
	ProtocolOpenAICompletions     MessageProtocol = "openai-completions"
	ProtocolAnthropicMessages     MessageProtocol = "anthropic-messages"

	ProviderGoogle    Provider = "google"
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"

	InputTypeText  InputType = "text"
	InputTypeImage InputType = "image"
)
