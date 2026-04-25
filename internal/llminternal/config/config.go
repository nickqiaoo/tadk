package config

import (
	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/tool"
)

type Config struct {
	Model   model.ModelAdapter
	ModelID string

	Tools    []tool.Tool
	Toolsets []tool.Toolset

	IncludeContents string

	Config *model.GenerateConfig

	Instruction               string
	InstructionProvider       InstructionProvider
	GlobalInstruction         string
	GlobalInstructionProvider InstructionProvider

	DisallowTransferToParent bool
	DisallowTransferToPeers  bool

	InputSchema  map[string]any
	OutputSchema map[string]any
}

// InstructionProvider resolves an agent's instruction text. It is invoked
// exactly once per invocation, at prefix-build time (before any LLM call).
// The result is frozen into the invocation's Prefix and reused verbatim
// across every subsequent step, which is what lets LLM-provider prompt
// caches hit on the system prompt. Providers must therefore be
// deterministic with respect to any per-step state; use the ReadonlyContext
// only for init-time reads (session metadata, user info).
type InstructionProvider func(ctx agent.InvocationContext) (string, error)

type SchemaCarrier interface {
	InputSchemaConfig() map[string]any
	OutputSchemaConfig() map[string]any
}

type ToolCarrier interface {
	DeclaredTools() []tool.Tool
}

// LLMConfigProvider is implemented by agents that expose their LLM configuration.
type LLMConfigProvider interface {
	LLMConfig() *Config
}

func (c *Config) DeclaredTools() []tool.Tool {
	return c.Tools
}

func (c *Config) InputSchemaConfig() map[string]any {
	return c.InputSchema
}

func (c *Config) OutputSchemaConfig() map[string]any {
	return c.OutputSchema
}

func (c *Config) TransferToParentDisabled() bool {
	return c.DisallowTransferToParent
}

// RootConfig returns the LLM Config of the root agent on the current
// invocation's parent chain, or nil if the caller is itself the root, the
// root is not an LLM agent, or the parent map is unavailable. Callers should
// fall back to their own Config when a root-only semantic (e.g.
// GlobalInstruction) is not present.
func RootConfig(ctx agent.InvocationContext) *Config {
	m := ctx.Tree()
	if m == nil {
		return nil
	}
	parent := m.ParentOf(ctx.AgentName())
	if parent == nil {
		// Current agent has no parent entry — it is the root itself.
		return nil
	}
	for {
		next := m.ParentOf(parent.Name())
		if next == nil {
			break
		}
		parent = next
	}
	if provider, ok := parent.(LLMConfigProvider); ok {
		return provider.LLMConfig()
	}
	return nil
}
