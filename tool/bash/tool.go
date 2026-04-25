// Package bashtool provides a tool to execute shell commands.
package bashtool

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/nickqiaoo/tadk/tool"
)

const (
	bashName        = "bash"
	bashDescription = "Executes a given bash command in a persistent shell session. Use this to run shell commands, scripts, or any CLI tools. Be careful with destructive commands."
	defaultTimeout  = 120 * time.Second
)

// New creates a new bash execution tool.
func New() *bashTool {
	return &bashTool{}
}

type bashTool struct{}

func (t *bashTool) Name() string        { return bashName }
func (t *bashTool) Description() string { return bashDescription }

func (t *bashTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	cmdStr, ok := args["command"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("missing required parameter: command")
	}

	timeout := defaultTimeout
	if t, ok := args["timeout"].(string); ok {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	execCtx, cancel := context.WithTimeout(ctx.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", cmdStr)
	output, err := cmd.CombinedOutput()

	result := map[string]any{
		"exit_code": cmd.ProcessState.ExitCode(),
		"output":    string(output),
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result["error"] = fmt.Sprintf("command timed out after %s", timeout)
		} else {
			result["error"] = err.Error()
		}
	}

	// Truncate output if too large
	const maxOutput = 10000
	if len(result["output"].(string)) > maxOutput {
		result["output"] = result["output"].(string)[:maxOutput] + "\n... (truncated)"
		result["truncated"] = true
	}

	return result, nil, nil
}

// Schema returns the JSON Schema for the tool's input parameters.
func (t *bashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute.",
			},
			"timeout": map[string]any{
				"type":        "string",
				"description": "Optional timeout duration (e.g. '30s', '5m'). Defaults to 120s.",
			},
		},
		"required": []string{"command"},
	}
}
