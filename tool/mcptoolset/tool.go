package mcptoolset

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nickqiaoo/tadk/tool"
)

func convertTool(t *mcp.Tool, client MCPClient) (tool.Tool, error) {
	mcp := &mcpTool{
		name:        t.Name,
		description: t.Description,
		mcpClient:   client,
	}

	// Since t.InputSchema and t.OutputSchema are pointers (*jsonschema.Schema) and the destination ResponseJsonSchema
	// is an interface (any), we have encountered the type nil problem.
	// This will make the omitempty not work since ResponseJsonSchema becomes an interface wrapper
	// to a nil pointer and genai converter includes "responseJsonSchema": null in the json sent to the llm which causes it to crash.
	// we need the following "if" check to keep ResponseJsonSchema (nil,nil) instead of (*jsonschema.Schema, nil)
	mcp.schema = schemaAnyToMap(t.InputSchema)
	return mcp, nil
}

type mcpTool struct {
	name        string
	description string
	schema      map[string]any

	mcpClient MCPClient
}

// Name implements the tool.Tool.
func (t *mcpTool) Name() string {
	return t.name
}

// Description implements the tool.Tool.
func (t *mcpTool) Description() string {
	return t.description
}

func (t *mcpTool) Schema() map[string]any {
	return t.schema
}

func (t *mcpTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	// TODO: add auth
	res, err := t.mcpClient.CallTool(ctx.Context(), &mcp.CallToolParams{
		Name:      t.name,
		Arguments: args,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to call MCP tool %q with err: %w", t.name, err)
	}

	if res.IsError {
		details := strings.Builder{}
		for _, c := range res.Content {
			textContent, ok := c.(*mcp.TextContent)
			if !ok {
				continue
			}
			if _, err := details.WriteString(textContent.Text); err != nil {
				return nil, nil, fmt.Errorf("failed to write error details: %w", err)
			}
		}

		errMsg := "Tool execution failed."
		if details.Len() > 0 {
			errMsg += " Details: " + details.String()
		}

		return nil, nil, errors.New(errMsg)
	}

	if res.StructuredContent != nil {
		return map[string]any{
			"output": res.StructuredContent,
		}, nil, nil
	}

	textResponse := strings.Builder{}

	for _, c := range res.Content {
		textContent, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}

		if _, err := textResponse.WriteString(textContent.Text); err != nil {
			return nil, nil, fmt.Errorf("failed to write text response: %w", err)
		}
	}

	if textResponse.Len() == 0 {
		return nil, nil, errors.New("no text content in tool response")
	}

	return map[string]any{
		"output": textResponse.String(),
	}, nil, nil
}

func schemaAnyToMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
