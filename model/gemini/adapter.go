package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/version"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	modelcompat "github.com/nickqiaoo/tadk/model/internal/compat"
)

// adapter implements model.ModelAdapter for Gemini models.
// It uses the official google.golang.org/genai SDK and handles
// Message ↔ genai.Content conversions internally.
type adapter struct {
	client             *genai.Client
	versionHeaderValue string
}

// Config holds configuration for the Gemini adapter.
type Config struct {
	ClientConfig *genai.ClientConfig
}

const thoughtSignatureBase64Prefix = "adk-b64:"

// New returns a model.ModelAdapter backed by the Gemini API.
func New(ctx context.Context, cfg Config) (model.ModelAdapter, error) {
	clientCfg := cfg.ClientConfig
	if clientCfg == nil {
		clientCfg = &genai.ClientConfig{}
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, err
	}

	headerValue := fmt.Sprintf("google-adk/%s gl-go/%s",
		version.Version, strings.TrimPrefix(runtime.Version(), "go"))

	return &adapter{
		client:             client,
		versionHeaderValue: headerValue,
	}, nil
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
	contents, config := a.convertRequest(req)
	a.addHeaders(config)
	maybeAppendUserContent(&contents)
	var finalMsg *message.Message
	var finalErr error
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {
			resolvedModel := a.resolvedModel(req)
			builder := model.NewAssistantEventBuilder()
			if ev := builder.Start(); ev != nil {
				if !yield(ev, nil) {
					return
				}
			}
			var lastMeta *message.Message
			for chunk, err := range a.client.Models.GenerateContentStream(ctx, resolvedModel, contents, config) {
				if err != nil {
					finalErr = fmt.Errorf("gemini: stream error: %w", err)
					yield(nil, finalErr)
					return
				}
				lastMeta = a.convertGenaiResponse(chunk, resolvedModel)
				for _, ev := range a.streamEvents(builder, chunk) {
					if !yield(ev, nil) {
						return
					}
				}
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
			finalMessage := builder.PartialMessage()
			if finalMessage == nil {
				finalMessage = &message.Message{Role: message.RoleAssistant}
			}
			if lastMeta != nil {
				finalMessage.Protocol = lastMeta.Protocol
				finalMessage.Provider = lastMeta.Provider
				finalMessage.Model = lastMeta.Model
				finalMessage.ResponseID = lastMeta.ResponseID
				finalMessage.Usage = lastMeta.Usage
				finalMessage.StopReason = lastMeta.StopReason
				finalMessage.ErrorCode = lastMeta.ErrorCode
				finalMessage.ErrorMessage = lastMeta.ErrorMessage
				finalMessage.Timestamp = lastMeta.Timestamp
			}
			finalMsg = finalMessage
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

func (a *adapter) streamEvents(builder *model.AssistantEventBuilder, resp *genai.GenerateContentResponse) []event.Event {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0] == nil || resp.Candidates[0].Content == nil {
		return nil
	}
	var events []event.Event
	for _, part := range resp.Candidates[0].Content.Parts {
		switch {
		case part.Text != "" && part.Thought:
			events = append(events, builder.ThinkingDelta(part.Text, string(part.ThoughtSignature), false)...)
		case part.Text != "":
			events = append(events, builder.TextDelta(part.Text)...)
		case part.FunctionCall != nil:
			events = append(events, builder.ToolCall(part.FunctionCall.ID, part.FunctionCall.Name, part.FunctionCall.Args, string(part.ThoughtSignature))...)
		}
	}
	return events
}

// convertRequest converts a canonical Request to genai contents + config.
func (a *adapter) convertRequest(req *model.Request) ([]*genai.Content, *genai.GenerateContentConfig) {
	config := &genai.GenerateContentConfig{}
	cfg := a.mergedConfig(req)

	if req.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		}
	}

	if len(req.Tools) > 0 {
		config.Tools = []*genai.Tool{convertTools(req.Tools)}
	}

	if cfg != nil {
		if cfg.Temperature != 0 {
			t := float32(cfg.Temperature)
			config.Temperature = &t
		}
		if cfg.TopP != 0 {
			p := float32(cfg.TopP)
			config.TopP = &p
		}
		if cfg.TopK > 0 {
			k := float32(cfg.TopK)
			config.TopK = &k
		}
		if cfg.MaxOutputTokens > 0 {
			config.MaxOutputTokens = int32(cfg.MaxOutputTokens)
		}
		if len(cfg.StopSequences) > 0 {
			config.StopSequences = cfg.StopSequences
		}
		if cfg.ResponseMIMEType != "" {
			config.ResponseMIMEType = cfg.ResponseMIMEType
		}
		if cfg.ResponseSchema != nil {
			config.ResponseJsonSchema = cfg.ResponseSchema
		}
		if cfg.CandidateCount > 0 {
			config.CandidateCount = int32(cfg.CandidateCount)
		}
		if cfg.Seed != 0 {
			seed := int32(cfg.Seed)
			config.Seed = &seed
		}
		if tc := geminiToolConfig(cfg); tc != nil {
			config.ToolConfig = tc
		}
		if thinking := geminiThinkingConfig(cfg); thinking != nil {
			config.ThinkingConfig = thinking
		}
		if labels := geminiLabels(cfg); len(labels) > 0 {
			config.Labels = labels
		}
		if cfg.ProviderOptions != nil {
			if config.HTTPOptions == nil {
				config.HTTPOptions = &genai.HTTPOptions{}
			}
			config.HTTPOptions.ExtraBody = model.CloneAnyMap(cfg.ProviderOptions)
		}
	}
	geminiApplyRequestOptions(config, cfg)

	var contents []*genai.Content
	targetMessages := modelcompat.TransformMessagesForModel(
		req.Messages,
		a.resolvedModel(req),
		model.ProtocolGeminiGenerateContent,
		model.ProviderGoogle,
		nil,
	)
	resolvedModel := a.resolvedModel(req)
	for _, msg := range targetMessages {
		if c := a.messageToGenai(msg, resolvedModel); c != nil {
			contents = append(contents, c)
		}
	}

	return contents, config
}

func (a *adapter) addHeaders(config *genai.GenerateContentConfig) {
	if config.HTTPOptions == nil {
		config.HTTPOptions = &genai.HTTPOptions{}
	}
	if config.HTTPOptions.Headers == nil {
		config.HTTPOptions.Headers = make(http.Header)
	}
	config.HTTPOptions.Headers.Set("x-goog-api-client", a.versionHeaderValue)
	config.HTTPOptions.Headers.Set("user-agent", a.versionHeaderValue)
}

func geminiApplyRequestOptions(config *genai.GenerateContentConfig, cfg *model.GenerateConfig) {
	if config == nil || cfg == nil {
		return
	}
	if config.HTTPOptions == nil {
		config.HTTPOptions = &genai.HTTPOptions{}
	}
	if config.HTTPOptions.Headers == nil {
		config.HTTPOptions.Headers = make(http.Header)
	}
	for k, v := range cfg.Headers {
		config.HTTPOptions.Headers.Set(k, v)
	}
	if cfg.MaxRetryDelayMs > 0 {
		d := time.Duration(cfg.MaxRetryDelayMs) * time.Millisecond
		config.HTTPOptions.Timeout = &d
	}
}

func geminiToolConfig(cfg *model.GenerateConfig) *genai.ToolConfig {
	if cfg == nil || cfg.ToolChoice == nil {
		return nil
	}
	fc := &genai.FunctionCallingConfig{}
	switch cfg.ToolChoice.Mode {
	case model.ToolChoiceNone:
		fc.Mode = genai.FunctionCallingConfigModeNone
	case model.ToolChoiceRequired:
		fc.Mode = genai.FunctionCallingConfigModeAny
	case model.ToolChoiceTool:
		fc.Mode = genai.FunctionCallingConfigModeAny
		if cfg.ToolChoice.Name != "" {
			fc.AllowedFunctionNames = []string{cfg.ToolChoice.Name}
		}
	default:
		fc.Mode = genai.FunctionCallingConfigModeAuto
	}
	if cfg.ParallelToolCalls {
		streamArgs := true
		fc.StreamFunctionCallArguments = &streamArgs
	}
	return &genai.ToolConfig{FunctionCallingConfig: fc}
}

func geminiThinkingConfig(cfg *model.GenerateConfig) *genai.ThinkingConfig {
	if cfg == nil || cfg.ThinkingLevel == "" || cfg.ThinkingLevel == "off" {
		return nil
	}
	out := &genai.ThinkingConfig{IncludeThoughts: true}
	if level := geminiThinkingLevel(cfg.ThinkingLevel); level != "" {
		out.ThinkingLevel = level
	}
	if budget := model.ConfiguredThinkingBudget(cfg.ThinkingBudgets, cfg.ThinkingLevel); budget > 0 {
		b := int32(budget)
		out.ThinkingBudget = &b
	}
	return out
}

func geminiThinkingLevel(level string) genai.ThinkingLevel {
	switch level {
	case "minimal":
		return genai.ThinkingLevelMinimal
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	case "high":
		return genai.ThinkingLevelHigh
	case "xhigh":
		return genai.ThinkingLevelHigh
	default:
		return ""
	}
}

func geminiLabels(cfg *model.GenerateConfig) map[string]string {
	labels := model.StringMap(cfg.Metadata)
	if cfg.SessionID != "" {
		if labels == nil {
			labels = make(map[string]string)
		}
		if _, exists := labels["session_id"]; !exists {
			labels["session_id"] = cfg.SessionID
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// messageToGenai converts a canonical Message to a genai.Content.
func (a *adapter) messageToGenai(msg *message.Message, targetModel string) *genai.Content {
	role := messageRoleToGenai(msg.Role)
	var parts []*genai.Part

	sameModel := msg != nil && msg.Role == message.RoleAssistant &&
		msg.Protocol == string(model.ProtocolGeminiGenerateContent) &&
		msg.Provider == string(model.ProviderGoogle) &&
		msg.Model == targetModel
	if sameModel && !geminiReplaySafe(msg) {
		sameModel = false
	}
	isGemini3 := strings.Contains(strings.ToLower(targetModel), "gemini-3")

	for _, part := range msg.Content {
		switch c := part.(type) {
		case *message.TextContent:
			if strings.TrimSpace(c.Text) == "" {
				continue
			}
			out := &genai.Part{Text: c.Text}
			if sameModel {
				if sig, ok := decodeThoughtSignatureString(c.TextSignature); ok && len(sig) > 0 {
					out.ThoughtSignature = sig
				}
			}
			parts = append(parts, out)

		case *message.ThinkingContent:
			if strings.TrimSpace(c.Thinking) == "" && c.ThinkingSignature == "" {
				continue
			}
			if !sameModel {
				if strings.TrimSpace(c.Thinking) != "" {
					parts = append(parts, &genai.Part{Text: c.Thinking})
				}
				continue
			}
			out := &genai.Part{
				Text:    c.Thinking,
				Thought: true,
			}
			if sig, ok := decodeThoughtSignatureString(c.ThinkingSignature); ok && len(sig) > 0 {
				out.ThoughtSignature = sig
			}
			parts = append(parts, out)

		case *message.ToolCallContent:
			out := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   c.ID,
					Name: c.Name,
					Args: c.Arguments,
				},
			}
			if sameModel {
				if sig, ok := decodeThoughtSignatureString(c.ThoughtSignature); ok && len(sig) > 0 {
					out.ThoughtSignature = sig
				}
			} else if isGemini3 {
				out.ThoughtSignature = []byte("skip_thought_signature_validator")
			}
			parts = append(parts, out)

		case *message.ImageContent:
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: c.MimeType,
					Data:     []byte(c.Data),
				},
			})
		}
	}

	for _, tr := range msg.ToolResults() {
		var response map[string]any
		if m, ok := tr.Content.(map[string]any); ok {
			response = m
		} else {
			response = map[string]any{"output": tr.Content}
		}
		if tr.IsError {
			response = map[string]any{"error": tr.Content}
		}
		parts = append(parts, &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				ID:       tr.CallID,
				Name:     firstNonEmpty(tr.Name, tr.CallID),
				Response: response,
			},
		})
	}

	if len(parts) == 0 {
		return nil
	}
	return &genai.Content{Role: role, Parts: parts}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func messageRoleToGenai(role message.Role) string {
	switch role {
	case message.RoleUser, message.RoleToolResult:
		return "user"
	case message.RoleAssistant:
		return "model"
	default:
		return "user"
	}
}

// convertTools converts canonical ToolDefinitions to a single genai.Tool.
func convertTools(tools []model.ToolDefinition) *genai.Tool {
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: t.Parameters,
		})
	}
	return &genai.Tool{FunctionDeclarations: decls}
}

// convertGenaiResponse converts a genai.GenerateContentResponse to a canonical assistant message.
func (a *adapter) convertGenaiResponse(resp *genai.GenerateContentResponse, modelID string) *message.Message {
	if len(resp.Candidates) == 0 {
		errCode := "UNKNOWN_ERROR"
		errMsg := "no candidates in response"
		if resp.PromptFeedback != nil {
			errCode = string(resp.PromptFeedback.BlockReason)
			errMsg = resp.PromptFeedback.BlockReasonMessage
		}
		return &message.Message{
			Role:         message.RoleAssistant,
			Protocol:     string(model.ProtocolGeminiGenerateContent),
			Provider:     string(model.ProviderGoogle),
			Model:        modelID,
			ResponseID:   resp.ResponseID,
			StopReason:   message.StopReasonError,
			ErrorCode:    errCode,
			ErrorMessage: errMsg,
			Usage:        convertUsage(resp.UsageMetadata),
		}
	}

	candidate := resp.Candidates[0]

	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return &message.Message{
			Role:         message.RoleAssistant,
			Protocol:     string(model.ProtocolGeminiGenerateContent),
			Provider:     string(model.ProviderGoogle),
			Model:        modelID,
			ResponseID:   resp.ResponseID,
			StopReason:   convertFinishReason(candidate.FinishReason),
			ErrorCode:    string(candidate.FinishReason),
			ErrorMessage: candidate.FinishMessage,
			Usage:        convertUsage(resp.UsageMetadata),
		}
	}

	msg := genaiContentToMessage(candidate.Content)
	msg.Protocol = string(model.ProtocolGeminiGenerateContent)
	msg.Provider = string(model.ProviderGoogle)
	msg.Model = modelID
	msg.ResponseID = resp.ResponseID
	msg.StopReason = convertFinishReason(candidate.FinishReason)
	msg.Usage = convertUsage(resp.UsageMetadata)
	return msg
}

// genaiContentToMessage converts a genai.Content to a canonical Message.
func genaiContentToMessage(content *genai.Content) *message.Message {
	role := genaiRoleToMessage(content.Role)
	var parts []message.Content

	for _, part := range content.Parts {
		switch {
		case part.Text != "" && part.Thought:
			parts = append(parts, &message.ThinkingContent{
				Thinking:          part.Text,
				ThinkingSignature: encodeThoughtSignatureBytes(part.ThoughtSignature),
			})

		case part.Text != "":
			parts = append(parts, &message.TextContent{
				Text:          part.Text,
				TextSignature: encodeThoughtSignatureBytes(part.ThoughtSignature),
			})

		case part.FunctionCall != nil:
			parts = append(parts, &message.ToolCallContent{
				ID:               part.FunctionCall.ID,
				Name:             part.FunctionCall.Name,
				Arguments:        part.FunctionCall.Args,
				ThoughtSignature: encodeThoughtSignatureBytes(part.ThoughtSignature),
			})

		case part.FunctionResponse != nil:
			return message.NewToolResultMessages([]message.ToolResult{{
				CallID:  part.FunctionResponse.ID,
				Name:    part.FunctionResponse.Name,
				Content: part.FunctionResponse.Response,
				IsError: false,
			}}, nil)

		case part.InlineData != nil:
			parts = append(parts, &message.ImageContent{
				MimeType: part.InlineData.MIMEType,
				Data:     string(part.InlineData.Data),
			})
		}
	}
	return &message.Message{Role: role, Content: parts}
}

func genaiRoleToMessage(role string) message.Role {
	switch role {
	case "user":
		return message.RoleUser
	case "model":
		return message.RoleAssistant
	default:
		return message.RoleUser
	}
}

func encodeThoughtSignatureBytes(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return thoughtSignatureBase64Prefix + base64.StdEncoding.EncodeToString(raw)
}

func decodeThoughtSignatureString(raw string) ([]byte, bool) {
	if raw == "" {
		return nil, true
	}
	if strings.HasPrefix(raw, thoughtSignatureBase64Prefix) {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, thoughtSignatureBase64Prefix))
		if err != nil {
			return nil, false
		}
		return decoded, true
	}
	if strings.ContainsRune(raw, '\uFFFD') {
		return nil, false
	}
	return []byte(raw), true
}

func geminiReplaySafe(msg *message.Message) bool {
	if msg == nil {
		return true
	}
	for _, part := range msg.Content {
		switch c := part.(type) {
		case *message.TextContent:
			if _, ok := decodeThoughtSignatureString(c.TextSignature); !ok {
				return false
			}
		case *message.ThinkingContent:
			if _, ok := decodeThoughtSignatureString(c.ThinkingSignature); !ok {
				return false
			}
		case *message.ToolCallContent:
			if _, ok := decodeThoughtSignatureString(c.ThoughtSignature); !ok {
				return false
			}
		}
	}
	return true
}

func convertUsage(u *genai.GenerateContentResponseUsageMetadata) *model.Usage {
	if u == nil {
		return nil
	}
	return &model.Usage{
		InputTokens:     int(u.PromptTokenCount),
		OutputTokens:    int(u.CandidatesTokenCount),
		CacheReadTokens: int(u.CachedContentTokenCount),
		TotalTokens:     int(u.TotalTokenCount),
	}
}

func convertFinishReason(reason genai.FinishReason) model.StopReason {
	switch string(reason) {
	case "STOP":
		return model.StopReasonStop
	case "MAX_TOKENS":
		return model.StopReasonLength
	case "SAFETY":
		return model.StopReasonSafety
	case "":
		return model.StopReasonUnknown
	default:
		return model.StopReasonUnknown
	}
}

// maybeAppendUserContent ensures the last content is a user message,
// which Gemini requires to continue generation.
func maybeAppendUserContent(contents *[]*genai.Content) {
	if len(*contents) == 0 {
		*contents = append(*contents, genai.NewContentFromText(
			"Handle the requests as specified in the System Instruction.", "user"))
		return
	}
	if last := (*contents)[len(*contents)-1]; last.Role != "user" {
		*contents = append(*contents, genai.NewContentFromText(
			"Continue processing previous requests as instructed.", "user"))
	}
}
