package utils

import (
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

// TransferTarget describes an agent handoff target exposed to the model.
type TransferTarget struct {
	Name        string
	Description string
}

// BuildHistory converts durable session entries into LLM message history.
func BuildHistory(entries []session.DurableEntry, branch string) []*message.Message {
	var history []*message.Message
	for _, entry := range entries {
		msgEntry, ok := entry.(*session.MessageEntry)
		if !ok || msgEntry == nil || msgEntry.Message == nil {
			continue
		}
		if !belongsToBranch(branch, msgEntry.Branch) {
			continue
		}
		history = append(history, message.Clone(msgEntry.Message))
	}
	return history
}

// BuildToolDefinitions converts local tools into provider-neutral tool schemas.
func BuildToolDefinitions(tools []tool.Tool) ([]model.ToolDefinition, map[string]tool.Tool) {
	toolsMap := make(map[string]tool.Tool, len(tools))
	defs := make([]model.ToolDefinition, 0, len(tools))

	for _, t := range tools {
		if t == nil {
			continue
		}
		toolsMap[t.Name()] = t

		defs = append(defs, model.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return defs, toolsMap
}

// BuildTransferToolDefinitions exposes handoff targets using one tool per target.
func BuildTransferToolDefinitions(targets []TransferTarget) []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(targets))
	for _, target := range targets {
		if target.Name == "" {
			continue
		}
		defs = append(defs, model.ToolDefinition{
			Name:        TransferToolPrefix + target.Name,
			Description: fmt.Sprintf("Transfer to %s: %s", target.Name, target.Description),
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}
	return defs
}

func belongsToBranch(targetBranch, eventBranch string) bool {
	if targetBranch == "" || eventBranch == "" {
		return true
	}
	if eventBranch == targetBranch {
		return true
	}
	return strings.HasPrefix(targetBranch, eventBranch+".")
}

