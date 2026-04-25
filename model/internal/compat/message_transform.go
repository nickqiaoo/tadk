// Package compat contains provider adapter replay compatibility helpers.
package compat

import (
	"strings"
	"time"

	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
)

// ToolCallIDNormalizer adapts historical tool-call IDs to a target provider's
// validation rules. It mirrors PI's transformMessages hook.
type ToolCallIDNormalizer func(id string, protocol model.MessageProtocol, provider model.Provider, modelID string, source *message.Message) string

// TransformMessagesForModel normalizes persisted canonical messages before
// replaying them to a specific model/provider. The behavior follows PI's
// transformMessages.
func TransformMessagesForModel(
	messages []*message.Message,
	modelID string,
	protocol model.MessageProtocol,
	provider model.Provider,
	normalize ToolCallIDNormalizer,
) []*message.Message {
	if len(messages) == 0 {
		return nil
	}
	transformed := make([]*message.Message, 0, len(messages))
	toolCallIDMap := make(map[string]string)

	for _, msg := range messages {
		msg = message.Clone(msg)
		if msg == nil {
			continue
		}
		switch msg.Role {
		case message.RoleUser:
			transformed = append(transformed, msg)
		case message.RoleToolResult:
			transformed = append(transformed, transformToolResultIDs(msg, toolCallIDMap))
		case message.RoleAssistant:
			sameModel := sameProviderModel(msg, modelID, protocol, provider)
			msg.Content = transformAssistantContent(msg, sameModel, normalize, toolCallIDMap, protocol, provider, modelID)
			transformed = append(transformed, msg)
		default:
			transformed = append(transformed, msg)
		}
	}

	out := make([]*message.Message, 0, len(transformed))
	var pendingToolCalls []message.ToolCall
	existingToolResults := make(map[string]bool)

	flushOrphaned := func() {
		for _, tc := range pendingToolCalls {
			if existingToolResults[tc.ID] {
				continue
			}
			out = append(out, message.NewToolResultMessages([]message.ToolResult{{
				CallID:  tc.ID,
				Name:    tc.Name,
				Content: "No result provided",
				IsError: true,
			}}, []message.Content{&message.TextContent{Text: "No result provided"}}))
			out[len(out)-1].Timestamp = time.Now().UnixMilli()
		}
		pendingToolCalls = nil
		existingToolResults = make(map[string]bool)
	}

	for _, msg := range transformed {
		switch msg.Role {
		case message.RoleAssistant:
			if len(pendingToolCalls) > 0 {
				flushOrphaned()
			}
			if msg.StopReason == message.StopReasonError || msg.StopReason == message.StopReasonAborted {
				continue
			}
			toolCalls := msg.ToolCalls()
			if len(toolCalls) > 0 {
				pendingToolCalls = toolCalls
				existingToolResults = make(map[string]bool)
			}
			out = append(out, msg)
		case message.RoleToolResult:
			for _, tr := range msg.ToolResults() {
				if tr.CallID != "" {
					existingToolResults[tr.CallID] = true
				}
			}
			out = append(out, msg)
		case message.RoleUser:
			if len(pendingToolCalls) > 0 {
				flushOrphaned()
			}
			out = append(out, msg)
		default:
			out = append(out, msg)
		}
	}

	return out
}

func sameProviderModel(msg *message.Message, modelID string, protocol model.MessageProtocol, provider model.Provider) bool {
	if msg == nil || msg.Role != message.RoleAssistant {
		return false
	}
	return msg.Protocol == string(protocol) &&
		msg.Provider == string(provider) &&
		msg.Model == modelID
}

func transformAssistantContent(
	msg *message.Message,
	sameModel bool,
	normalize ToolCallIDNormalizer,
	idMap map[string]string,
	protocol model.MessageProtocol,
	provider model.Provider,
	modelID string,
) []message.Content {
	var out []message.Content
	for _, block := range msg.Content {
		switch c := block.(type) {
		case *message.ThinkingContent:
			if c.Redacted {
				if sameModel {
					out = append(out, c)
				}
				continue
			}
			if sameModel && c.ThinkingSignature != "" {
				out = append(out, c)
				continue
			}
			if strings.TrimSpace(c.Thinking) == "" {
				continue
			}
			if sameModel {
				out = append(out, c)
			} else {
				out = append(out, &message.TextContent{Text: c.Thinking})
			}
		case *message.TextContent:
			if sameModel {
				out = append(out, c)
			} else {
				out = append(out, &message.TextContent{Text: c.Text})
			}
		case *message.ToolCallContent:
			next := *c
			if !sameModel {
				next.ThoughtSignature = ""
			}
			if !sameModel && normalize != nil && c.ID != "" {
				normalized := normalize(c.ID, protocol, provider, modelID, msg)
				if normalized != "" && normalized != c.ID {
					idMap[c.ID] = normalized
					next.ID = normalized
				}
			}
			out = append(out, &next)
		default:
			out = append(out, block)
		}
	}
	return out
}

func transformToolResultIDs(msg *message.Message, idMap map[string]string) *message.Message {
	if msg == nil || len(idMap) == 0 {
		return msg
	}
	for i, tr := range msg.ToolResultsData {
		if normalized := idMap[tr.CallID]; normalized != "" {
			tr.CallID = normalized
			msg.ToolResultsData[i] = tr
		}
	}
	return msg
}
