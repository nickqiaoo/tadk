package processors

import (
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/agent"
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"
)

// ResolveInstructions composes the invariant system text from this agent's
// local Instruction (or InstructionProvider) and the root agent's
// GlobalInstruction (or GlobalInstructionProvider). Providers are invoked
// exactly once per invocation, at prefix-build time; they must not depend on
// per-step state or the LLM prompt cache will never hit.
func ResolveInstructions(ctx agent.InvocationContext, cfg *llmconfig.Config) (string, error) {
	rootCfg := llmconfig.RootConfig(ctx)
	if rootCfg == nil {
		rootCfg = cfg
	}
	global, err := resolveGlobalInstruction(ctx, rootCfg)
	if err != nil {
		return "", fmt.Errorf("resolve global instruction: %w", err)
	}
	local, err := resolveInstruction(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("resolve instruction: %w", err)
	}
	parts := make([]string, 0, 2)
	if global != "" {
		parts = append(parts, global)
	}
	if local != "" {
		parts = append(parts, local)
	}
	return strings.Join(parts, "\n\n"), nil
}

func resolveInstruction(ctx agent.InvocationContext, cfg *llmconfig.Config) (string, error) {
	if cfg.InstructionProvider != nil {
		return cfg.InstructionProvider(ctx)
	}
	return cfg.Instruction, nil
}

func resolveGlobalInstruction(ctx agent.InvocationContext, cfg *llmconfig.Config) (string, error) {
	if cfg.GlobalInstructionProvider != nil {
		return cfg.GlobalInstructionProvider(ctx)
	}
	return cfg.GlobalInstruction, nil
}

func InjectSessionState(ctx agent.InvocationContext, template string) (string, error) {
	return template, nil
}
