package processors

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/google/safehtml/template"

	"github.com/nickqiaoo/tadk/agent"
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"

	"github.com/nickqiaoo/tadk/tool"
)

// ResolveTransferContribution returns the system-prompt addendum and the
// transfer tool that should be exposed for the current agent's transfer
// targets. Both values are zero when the agent has no valid transfer
// targets. The result depends only on the agent tree and cfg, so it is
// stable for the lifetime of an invocation.
func ResolveTransferContribution(ctx agent.InvocationContext, ag agent.Agent, cfg *llmconfig.Config, tree agent.ParentTree) (string, tool.Tool, error) {
	if ag == nil {
		return "", nil, nil
	}
	if len(ag.SubAgents()) == 0 && cfg != nil && cfg.DisallowTransferToParent && cfg.DisallowTransferToPeers {
		return "", nil, nil
	}

	if tree == nil {
		return "", nil, nil
	}
	targets := TransferTargets(ag, cfg, tree.ParentOf(ag.Name()))
	if len(targets) == 0 {
		return "", nil, nil
	}

	transferTool := &TransferToAgentTool{}
	si, err := InstructionsForTransferToAgent(ag, cfg, tree.ParentOf(ag.Name()), targets, transferTool)
	if err != nil {
		return "", nil, err
	}
	return si, transferTool, nil
}

type TransferToAgentTool struct{}

func (t *TransferToAgentTool) Description() string {
	return `Transfer the question to another agent.
	This tool hands off control to another agent when it's more suitable to answer the user's question according to the agent's description.`
}

func (t *TransferToAgentTool) Name() string {
	return "transfer_to_agent"
}

func (t *TransferToAgentTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_name": map[string]any{
				"type":        "string",
				"description": "the agent name to transfer to",
			},
		},
		"required": []string{"agent_name"},
	}
}

func (t *TransferToAgentTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	if args == nil {
		return nil, nil, fmt.Errorf("missing argument")
	}
	agentName, ok := args["agent_name"].(string)
	if !ok || agentName == "" {
		return nil, nil, fmt.Errorf("empty agent_name: %v", args)
	}
	return map[string]any{}, &tool.Control{TransferToAgent: agentName}, nil
}

var _ tool.Tool = (*TransferToAgentTool)(nil)

func TransferTargets(curAgent agent.Agent, cfg *llmconfig.Config, parent agent.Agent) []agent.Agent {
	targets := slices.Clone(curAgent.SubAgents())
	if cfg == nil {
		return targets
	}
	if !cfg.DisallowTransferToParent && isLLMAgent(parent) {
		targets = append(targets, parent)
	}
	if !cfg.DisallowTransferToPeers && isLLMAgent(parent) {
		for _, peer := range parent.SubAgents() {
			if peer.Name() != curAgent.Name() {
				targets = append(targets, peer)
			}
		}
	}
	return targets
}

func isLLMAgent(ag agent.Agent) bool {
	return ag != nil && agent.TypeOf(ag) == agent.TypeLLMAgent
}

var transferToAgentPromptTmpl = template.Must(
	template.New("transfer_to_agent_prompt").Parse(agentTransferInstructionTemplate))

func InstructionsForTransferToAgent(curAgent agent.Agent, cfg *llmconfig.Config, parent agent.Agent, targets []agent.Agent, transferTool tool.Tool) (string, error) {
	if cfg != nil && cfg.DisallowTransferToParent {
		parent = nil
	}
	var buf bytes.Buffer
	if err := transferToAgentPromptTmpl.Execute(&buf, struct {
		AgentName string
		Parent    agent.Agent
		Targets   []agent.Agent
		ToolName  string
	}{
		AgentName: curAgent.Name(),
		Parent:    parent,
		Targets:   targets,
		ToolName:  transferTool.Name(),
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const agentTransferInstructionTemplate = `You have a list of other agents to transfer to:
{{range .Targets}}
Agent name: {{.Name}}
Agent description: {{.Description}}
{{end}}
If you are the best to answer the question according to your description, you
can answer it.
If another agent is better for answering the question according to its
description, call '{{.ToolName}}' function to transfer the
question to that agent. When transfering, do not generate any text other than
the function call.
{{if .Parent}}
Your parent agent is {{.Parent.Name}}. If neither the other agents nor
you are best for answering the question according to the descriptions, transfer
to your parent agent. If you don't have parent agent, try answer by yourself.
{{end}}
`
