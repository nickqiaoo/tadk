package llminternal

import (
	"fmt"

	"github.com/nickqiaoo/tadk/agent"
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"
	"github.com/nickqiaoo/tadk/internal/llminternal/processors"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/internal/utils"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/tool"
)

// Prefix is the invocation-scoped, immutable subset of an LLM request that
// can be reused across every step of a tool-call loop. Building it once and
// reusing it is what enables LLM-provider prompt caching (Anthropic
// cache_control, OpenAI prefix cache, Gemini context cache) to hit.
//
// Everything in Prefix must be byte-identical between steps. Per-step state
// (session history, new user input) is applied separately in
// Flow.preprocess.
type Prefix struct {
	ModelID      string
	GenConfig    *model.GenerateConfig
	SystemPrompt string
	Tools        []tool.Tool
	ToolDefs     []model.ToolDefinition
	ToolsMap     map[string]tool.Tool
}

// BuildPrefix resolves every static contributor (config, instructions,
// tools, agent-transfer prompt/tool, request augmenters) exactly once and
// returns a Prefix for the invocation. Providers and toolsets are invoked
// here and only here; any dynamic behavior inside them effectively freezes
// at this point, which is exactly what prompt caching requires.
func BuildPrefix(ctx agent.InvocationContext, ag agent.Agent, cfg *llmconfig.Config) (*Prefix, error) {
	if cfg == nil {
		return &Prefix{}, nil
	}

	modelID, gen := processors.ResolveBasic(cfg)

	tools, err := processors.CollectTools(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("collect tools: %w", err)
	}

	localAndGlobal, err := processors.ResolveInstructions(ctx, cfg)
	if err != nil {
		return nil, err
	}

	transferSys, transferTool, err := processors.ResolveTransferContribution(ctx, ag, cfg, ctx.Tree())
	if err != nil {
		return nil, fmt.Errorf("resolve transfer contribution: %w", err)
	}
	if transferTool != nil {
		tools = append(tools, transferTool)
	}

	// Run any RequestAugmenter contributions against a scratch request whose
	// SystemPrompt carries what we've composed so far. Augmenters may append
	// further system text; Tools/Messages/Config mutations are not honored
	// here — the prefix must be byte-stable, so any dynamic contribution
	// belongs on the per-step path.
	scratch := &model.Request{SystemPrompt: joinSystem(localAndGlobal, transferSys)}
	toolCtx := toolinternal.NewToolContext(ctx, "")
	for _, t := range tools {
		aug, ok := t.(toolinternal.RequestAugmenter)
		if !ok {
			continue
		}
		if err := aug.AugmentRequest(toolCtx, scratch); err != nil {
			return nil, fmt.Errorf("augment request for tool %q: %w", t.Name(), err)
		}
	}

	toolDefs, toolsMap := utils.BuildToolDefinitions(tools)

	return &Prefix{
		ModelID:      modelID,
		GenConfig:    gen,
		SystemPrompt: scratch.SystemPrompt,
		Tools:        tools,
		ToolDefs:     toolDefs,
		ToolsMap:     toolsMap,
	}, nil
}

func joinSystem(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" {
			out = p
			continue
		}
		out = out + "\n\n" + p
	}
	return out
}
