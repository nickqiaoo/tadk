// Package mcptoolset provides an MCP tool set.
package mcptoolset

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/tool"
)

// New returns MCP ToolSet.
// MCP ToolSet connects to a MCP Server, retrieves MCP Tools into ADK Tools and
// passes them to the LLM.
// It uses https://github.com/modelcontextprotocol/go-sdk for MCP communication.
// MCP session is created lazily on the first request to LLM.
//
// Usage: create MCP ToolSet with mcptoolset.New() and provide it to the
// LLMAgent in the llmagent.Config.
//
// Example:
//
//	llmagent.New(llmagent.Config{
//		Name:        "agent_name",
//		Model:       model,
//		Description: "...",
//		Instruction: "...",
//		Toolsets: []tool.Set{
//			mcptoolset.New(mcptoolset.Config{
//				Transport: &mcp.CommandTransport{Command: exec.Command("myserver")}
//			}),
//		},
//	})
func New(cfg Config) (tool.Toolset, error) {
	return &set{
		mcpClient:  newConnectionRefresher(cfg.Client, cfg.Transport),
		toolFilter: cfg.ToolFilter,
	}, nil
}

// Config provides initial configuration for the MCP ToolSet.
type Config struct {
	// Client is an optional custom MCP client to use. If nil, a default client will be created.
	Client *mcp.Client
	// Transport that will be used to connect to MCP server.
	Transport mcp.Transport
	// Deprecated: use tool.FilterToolset instead.
	// ToolFilter selects tools for which tool.Predicate returns true.
	// If ToolFilter is nil, then all tools are returned.
	// tool.StringPredicate can be convenient if there's a known fixed list of tool names.
	ToolFilter tool.Predicate

}

type set struct {
	mcpClient  MCPClient
	toolFilter tool.Predicate
}

func (*set) Name() string {
	return "mcp_tool_set"
}

func (*set) Description() string {
	return "Connects to a MCP Server, retrieves MCP Tools into ADK Tools."
}

func (*set) IsLongRunning() bool {
	return false
}

// Tools fetch MCP tools from the server, convert to adk tool.Tool and filter by name.
func (s *set) Tools(ctx agent.InvocationContext) ([]tool.Tool, error) {
	mcpTools, err := s.mcpClient.ListTools(ctx.Context())
	if err != nil {
		return nil, err
	}

	var adkTools []tool.Tool
	for _, mcpTool := range mcpTools {
		t, err := convertTool(mcpTool, s.mcpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to convert MCP tool %q to adk tool: %w", mcpTool.Name, err)
		}

		if s.toolFilter != nil && !s.toolFilter(ctx, t) {
			continue
		}

		adkTools = append(adkTools, t)
	}

	return adkTools, nil
}

