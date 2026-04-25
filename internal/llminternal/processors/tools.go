package processors

import (
	"fmt"

	"github.com/nickqiaoo/tadk/agent"
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"
	"github.com/nickqiaoo/tadk/tool"
)

func CollectTools(ctx agent.InvocationContext, cfg *llmconfig.Config) ([]tool.Tool, error) {
	tools := cfg.Tools
	for _, toolSet := range cfg.Toolsets {
		tsTools, err := toolSet.Tools(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to extract tools from the tool set %q: %w", toolSet.Name(), err)
		}
		tools = append(tools, tsTools...)
	}
	return tools, nil
}
