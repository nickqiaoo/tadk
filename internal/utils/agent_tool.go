package utils

import (
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/message"
)

const (
	// TransferToolPrefix is the Temporal target-specific transfer convention:
	// transfer_to_<agent-name>.
	TransferToolPrefix = "transfer_to_"
	// GenericTransferToolName is the ADK AutoFlow transfer convention that
	// carries the target agent name in the tool arguments.
	GenericTransferToolName = "transfer_to_agent"
)

// ToolCall is the provider-neutral local representation of a requested tool call.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ExtractToolCalls returns non-transfer tool calls from a model message.
func ExtractToolCalls(msg *message.Message) []ToolCall {
	if msg == nil {
		return nil
	}
	var calls []ToolCall
	for _, tc := range msg.ToolCalls() {
		if IsTargetTransferToolName(tc.Name) {
			continue
		}
		calls = append(calls, ToolCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: cloneAgentMap(tc.Args),
		})
	}
	return calls
}

// ExtractTransferTarget returns the agent name targeted by the first transfer tool call.
func ExtractTransferTarget(msg *message.Message) string {
	if msg == nil {
		return ""
	}
	for _, tc := range msg.ToolCalls() {
		target, ok := TargetFromTransferToolName(tc.Name)
		if ok {
			return target
		}
	}
	return ""
}

// IsTargetTransferToolName reports whether name follows the target-specific transfer convention.
func IsTargetTransferToolName(name string) bool {
	_, ok := TargetFromTransferToolName(name)
	return ok
}

// TargetFromTransferToolName extracts the agent target from a target-specific transfer tool name.
func TargetFromTransferToolName(name string) (string, bool) {
	if name == GenericTransferToolName || !strings.HasPrefix(name, TransferToolPrefix) {
		return "", false
	}
	target := strings.TrimPrefix(name, TransferToolPrefix)
	if target == "" {
		return "", false
	}
	return target, true
}

// ToolAddress returns the canonical address for a tool call within an agent address.
func ToolAddress(agentAddress, callID string) string {
	if agentAddress == "" {
		return fmt.Sprintf("tool:%s", callID)
	}
	return fmt.Sprintf("%s:tool:%s", agentAddress, callID)
}

// ResumeDataForToolCall finds resume data for a tool call.
// It accepts both the durable interrupt ID and legacy function-call ID keys.
func ResumeDataForToolCall(resumeData map[string]any, callID, interruptID string) (any, bool) {
	if resumeData == nil {
		return nil, false
	}
	if interruptID != "" {
		if data, ok := resumeData[interruptID]; ok {
			return data, true
		}
	}
	if data, ok := resumeData[callID]; ok {
		return data, true
	}
	return nil, false
}

// ToolResult creates a canonical tool result record and keeps the tool name.
func ToolResult(call ToolCall, content any, isError bool) message.ToolResult {
	return message.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
		IsError: isError,
	}
}

// ToolResultMessage creates a tool-result message from tool results and optional content.
func ToolResultMessage(results []message.ToolResult, content []message.Content) *message.Message {
	if len(results) == 0 && len(content) == 0 {
		return nil
	}
	return message.NewToolResultMessages(results, content)
}

func cloneAgentMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
