// Package toolinternal defines internal-only extension points for tools.
package toolinternal

import (
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/tool"
)

// RequestAugmenter lets a tool contribute to the invocation's static
// Prefix — for example, appending tool-specific instructions to the system
// prompt. It is invoked exactly once per invocation, at prefix-build time,
// against a scratch request. Implementations must only mutate SystemPrompt;
// Messages, Tools, and Config mutations are not propagated, because the
// Prefix must remain byte-identical across every step for LLM-provider
// prompt caches to hit. Anything that needs to vary per step belongs on
// the per-step Contents path, not here.
type RequestAugmenter interface {
	AugmentRequest(ctx tool.Context, req *model.Request) error
}
