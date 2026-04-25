package model

import "fmt"

func ConfiguredThinkingBudget(budgets *ThinkingBudgets, level string) int {
	if budgets == nil {
		return 0
	}
	switch level {
	case "minimal":
		return budgets.Minimal
	case "low":
		return budgets.Low
	case "medium":
		return budgets.Medium
	case "high":
		return budgets.High
	case "xhigh":
		if budgets.XHigh != 0 {
			return budgets.XHigh
		}
		return budgets.High
	default:
		return 0
	}
}

func DefaultThinkingBudgets() *ThinkingBudgets {
	return &ThinkingBudgets{}
}

func (b ThinkingBudgets) WithMinimal(v int) *ThinkingBudgets {
	b.Minimal = v
	return &b
}

func (b ThinkingBudgets) WithLow(v int) *ThinkingBudgets {
	b.Low = v
	return &b
}

func (b ThinkingBudgets) WithMedium(v int) *ThinkingBudgets {
	b.Medium = v
	return &b
}

func (b ThinkingBudgets) WithHigh(v int) *ThinkingBudgets {
	b.High = v
	return &b
}

func (b ThinkingBudgets) WithXHigh(v int) *ThinkingBudgets {
	b.XHigh = v
	return &b
}

func CloneThinkingBudgets(src *ThinkingBudgets) *ThinkingBudgets {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

func CloneToolChoice(src *ToolChoice) *ToolChoice {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

func CloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = cloneAnyValue(v)
	}
	return out
}

func cloneAnyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return CloneAnyMap(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = cloneAnyValue(item)
		}
		return out
	default:
		return v
	}
}

func CloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func MergeStringMap(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := CloneStringMap(base)
	if out == nil {
		out = make(map[string]string, len(override))
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func DefaultGenerateConfig() *GenerateConfig {
	return &GenerateConfig{
		Transport:      TransportAuto,
		CacheRetention: CacheRetentionNone,
	}
}

func (c GenerateConfig) WithTemperature(v float64) *GenerateConfig {
	c.Temperature = v
	return &c
}

func (c GenerateConfig) WithTopP(v float64) *GenerateConfig {
	c.TopP = v
	return &c
}

func (c GenerateConfig) WithTopK(v int) *GenerateConfig {
	c.TopK = v
	return &c
}

func (c GenerateConfig) WithMaxOutputTokens(v int) *GenerateConfig {
	c.MaxOutputTokens = v
	return &c
}

func (c GenerateConfig) WithStopSequences(v ...string) *GenerateConfig {
	c.StopSequences = append([]string(nil), v...)
	return &c
}

func (c GenerateConfig) WithSeed(v int) *GenerateConfig {
	c.Seed = v
	return &c
}

func (c GenerateConfig) WithThinkingLevel(v string) *GenerateConfig {
	c.ThinkingLevel = v
	return &c
}

func (c GenerateConfig) WithThinkingBudgets(v *ThinkingBudgets) *GenerateConfig {
	c.ThinkingBudgets = CloneThinkingBudgets(v)
	return &c
}

func (c GenerateConfig) WithResponseMIMEType(v string) *GenerateConfig {
	c.ResponseMIMEType = v
	return &c
}

func (c GenerateConfig) WithResponseSchema(v map[string]any) *GenerateConfig {
	c.ResponseSchema = CloneAnyMap(v)
	return &c
}

func (c GenerateConfig) WithHeaders(v map[string]string) *GenerateConfig {
	c.Headers = CloneStringMap(v)
	return &c
}

func (c GenerateConfig) WithHeader(key, value string) *GenerateConfig {
	c.Headers = MergeStringMap(c.Headers, map[string]string{key: value})
	return &c
}

func (c GenerateConfig) WithMetadata(v map[string]any) *GenerateConfig {
	c.Metadata = CloneAnyMap(v)
	return &c
}

func (c GenerateConfig) WithSessionID(v string) *GenerateConfig {
	c.SessionID = v
	return &c
}

func (c GenerateConfig) WithTransport(v Transport) *GenerateConfig {
	c.Transport = v
	return &c
}

func (c GenerateConfig) WithCacheRetention(v CacheRetention) *GenerateConfig {
	c.CacheRetention = v
	return &c
}

func (c GenerateConfig) WithMaxRetryDelayMs(v int) *GenerateConfig {
	c.MaxRetryDelayMs = v
	return &c
}

func (c GenerateConfig) WithToolChoice(v ToolChoice) *GenerateConfig {
	c.ToolChoice = CloneToolChoice(&v)
	return &c
}

func (c GenerateConfig) WithParallelToolCalls(v bool) *GenerateConfig {
	c.ParallelToolCalls = v
	return &c
}

func (c GenerateConfig) WithStore(v bool) *GenerateConfig {
	c.Store = v
	return &c
}

func (c GenerateConfig) WithServiceTier(v string) *GenerateConfig {
	c.ServiceTier = v
	return &c
}

func (c GenerateConfig) WithCandidateCount(v int) *GenerateConfig {
	c.CandidateCount = v
	return &c
}

func (c GenerateConfig) WithPromptCacheKey(v string) *GenerateConfig {
	c.PromptCacheKey = v
	return &c
}

func (c GenerateConfig) WithSafetyIdentifier(v string) *GenerateConfig {
	c.SafetyIdentifier = v
	return &c
}

func (c GenerateConfig) WithProviderOptions(v map[string]any) *GenerateConfig {
	c.ProviderOptions = CloneAnyMap(v)
	return &c
}

func CloneGenerateConfig(src *GenerateConfig) *GenerateConfig {
	if src == nil {
		return nil
	}
	out := *src
	out.StopSequences = append([]string(nil), src.StopSequences...)
	out.ThinkingBudgets = CloneThinkingBudgets(src.ThinkingBudgets)
	out.ResponseSchema = CloneAnyMap(src.ResponseSchema)
	out.Headers = CloneStringMap(src.Headers)
	out.Metadata = CloneAnyMap(src.Metadata)
	out.ToolChoice = CloneToolChoice(src.ToolChoice)
	out.ProviderOptions = CloneAnyMap(src.ProviderOptions)
	return &out
}

func ResolveGenerateConfig(base, override *GenerateConfig) *GenerateConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		return CloneGenerateConfig(override)
	}
	if override == nil {
		return CloneGenerateConfig(base)
	}

	out := CloneGenerateConfig(base)
	if override.Temperature != 0 {
		out.Temperature = override.Temperature
	}
	if override.TopP != 0 {
		out.TopP = override.TopP
	}
	if override.TopK > 0 {
		out.TopK = override.TopK
	}
	if override.MaxOutputTokens > 0 {
		out.MaxOutputTokens = override.MaxOutputTokens
	}
	if len(override.StopSequences) > 0 {
		out.StopSequences = append([]string(nil), override.StopSequences...)
	}
	if override.Seed != 0 {
		out.Seed = override.Seed
	}
	if override.ThinkingLevel != "" {
		out.ThinkingLevel = override.ThinkingLevel
	}
	if override.ThinkingBudgets != nil {
		out.ThinkingBudgets = CloneThinkingBudgets(override.ThinkingBudgets)
	}
	if override.ResponseMIMEType != "" {
		out.ResponseMIMEType = override.ResponseMIMEType
	}
	if override.ResponseSchema != nil {
		out.ResponseSchema = CloneAnyMap(override.ResponseSchema)
	}
	out.Headers = MergeStringMap(out.Headers, override.Headers)
	if override.Metadata != nil {
		out.Metadata = CloneAnyMap(override.Metadata)
	}
	if override.SessionID != "" {
		out.SessionID = override.SessionID
	}
	if override.Transport != "" {
		out.Transport = override.Transport
	}
	if override.CacheRetention != "" {
		out.CacheRetention = override.CacheRetention
	}
	if override.MaxRetryDelayMs > 0 {
		out.MaxRetryDelayMs = override.MaxRetryDelayMs
	}
	if override.ToolChoice != nil {
		out.ToolChoice = CloneToolChoice(override.ToolChoice)
	}
	if override.ParallelToolCalls {
		out.ParallelToolCalls = true
	}
	if override.Store {
		out.Store = true
	}
	if override.ServiceTier != "" {
		out.ServiceTier = override.ServiceTier
	}
	if override.CandidateCount > 0 {
		out.CandidateCount = override.CandidateCount
	}
	if override.PromptCacheKey != "" {
		out.PromptCacheKey = override.PromptCacheKey
	}
	if override.SafetyIdentifier != "" {
		out.SafetyIdentifier = override.SafetyIdentifier
	}
	if override.ProviderOptions != nil {
		out.ProviderOptions = CloneAnyMap(override.ProviderOptions)
	}
	return out
}

func StringMap(src map[string]any) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case string:
			out[k] = val
		case fmt.Stringer:
			out[k] = val.String()
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
			out[k] = fmt.Sprint(val)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
