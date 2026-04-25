package processors

import (
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/agent"
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/session"
)

func Contents(ctx agent.InvocationContext, req *model.Request, cfg *llmconfig.Config) error {
	fn := BuildMessagesDefault
	if cfg.IncludeContents == "none" {
		fn = BuildMessagesCurrentTurnContextOnly
	}
	var entries []*session.MessageEntry
	if ctx.Session() != nil {
		for entry := range ctx.Session().Entries().All() {
			if msgEntry, ok := entry.(*session.MessageEntry); ok {
				entries = append(entries, msgEntry)
			}
		}
	}
	msgs, err := fn(ctx.AgentName(), ctx.Branch(), entries)
	if err != nil {
		return err
	}
	req.Messages = append(req.Messages, msgs...)
	return nil
}

func BuildMessagesDefault(agentName, invocationBranch string, entries []*session.MessageEntry) ([]*message.Message, error) {
	var filtered []*session.MessageEntry
	for _, entry := range entries {
		msg := entry.Message
		if !messageForHistory(msg) {
			continue
		}
		if !entryBelongsToBranch(invocationBranch, entry) {
			continue
		}
		if isAuthEntry(entry) {
			continue
		}
		if isOtherAgentReply(agentName, entry) {
			filtered = append(filtered, ConvertForeignEntry(entry))
		} else {
			filtered = append(filtered, entry)
		}
	}

	filtered, err := pairToolCallsWithResponses(filtered)
	if err != nil {
		return nil, err
	}

	// req.Messages holds read-only views of session messages; each provider adapter
	// clones via TransformMessagesForModel before mutating.
	var msgs []*message.Message
	for _, entry := range filtered {
		if !messageForHistory(entry.Message) {
			continue
		}
		msgs = append(msgs, entry.Message)
	}
	return msgs, nil
}

func entryBelongsToBranch(invocationBranch string, entry *session.MessageEntry) bool {
	if invocationBranch == "" || entry.Branch == "" {
		return true
	}
	if entry.Branch == invocationBranch {
		return true
	}
	return strings.HasPrefix(invocationBranch, entry.Branch+".")
}

// pairToolCallsWithResponses ensures every tool call entry is immediately followed
// by a single merged entry containing all of its tool results.
func pairToolCallsWithResponses(entries []*session.MessageEntry) ([]*session.MessageEntry, error) {
	if len(entries) < 2 {
		return entries, nil
	}

	// Build a map from call ID to the response entries that contain it.
	callIDToResponses := make(map[string][]*session.MessageEntry)
	for _, entry := range entries {
		if entry.Message == nil {
			continue
		}
		for _, tr := range entry.Message.ToolResults() {
			callIDToResponses[tr.CallID] = append(callIDToResponses[tr.CallID], entry)
		}
	}

	// Track which response entries have already been emitted.
	emitted := make(map[*session.MessageEntry]bool)

	var result []*session.MessageEntry
	for _, entry := range entries {
		// Skip response entries that were already emitted after their call.
		if emitted[entry] {
			continue
		}

		result = append(result, entry)

		if entry.Message == nil || len(entry.Message.ToolCalls()) == 0 {
			continue
		}

		// Gather all response entries for this tool call.
		responseSet := make(map[*session.MessageEntry]struct{})
		for _, tc := range entry.Message.ToolCalls() {
			for _, respEntry := range callIDToResponses[tc.ID] {
				if respEntry != entry && !emitted[respEntry] {
					responseSet[respEntry] = struct{}{}
				}
			}
		}

		if len(responseSet) == 0 {
			continue
		}

		// Merge the response entries into a single entry.
		responses := make([]*session.MessageEntry, 0, len(responseSet))
		for resp := range responseSet {
			responses = append(responses, resp)
		}
		merged, err := mergeToolResponseEntries(responses)
		if err != nil {
			return nil, fmt.Errorf("failed to merge tool response entries: %w", err)
		}
		result = append(result, merged)

		for resp := range responseSet {
			emitted[resp] = true
		}
	}

	return result, nil
}

func mergeToolResponseEntries(toolResponseEntries []*session.MessageEntry) (*session.MessageEntry, error) {
	if len(toolResponseEntries) == 0 {
		return nil, fmt.Errorf("at least one tool_response entry is required")
	}

	first := toolResponseEntries[0]
	if first == nil {
		return nil, fmt.Errorf("merged entry based on the first entry should not be nil")
	}
	if first.Message == nil {
		return nil, fmt.Errorf("message for the first entry should not be nil")
	}
	mergedEntry := shallowCopyEntry(first)
	mergedResults := append([]message.ToolResult(nil), first.Message.ToolResults()...)
	mergedContent := append([]message.Content(nil), first.Message.Content...)
	partIndicesByCallID := make(map[string]int)
	for idx, tr := range mergedResults {
		partIndicesByCallID[tr.CallID] = idx
	}

	if len(partIndicesByCallID) == 0 {
		return nil, fmt.Errorf("there should be at least one tool result")
	}

	for _, entry := range toolResponseEntries[1:] {
		results := entry.Message.ToolResults()
		if entry.Message == nil || (len(results) == 0 && len(entry.Message.Content) == 0) {
			return nil, fmt.Errorf("entry should contain at least one tool result or content item")
		}

		mergedContent = append(mergedContent, entry.Message.Content...)
		for _, tr := range results {
			if idx, found := partIndicesByCallID[tr.CallID]; found {
				mergedResults[idx] = tr
			} else {
				mergedResults = append(mergedResults, tr)
				partIndicesByCallID[tr.CallID] = len(mergedResults) - 1
			}
		}
	}
	mergedEntry.Message = message.NewToolResultMessages(mergedResults, mergedContent)

	return mergedEntry, nil
}

func BuildMessagesCurrentTurnContextOnly(agentName, branch string, entries []*session.MessageEntry) ([]*message.Message, error) {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Author == session.AuthorUser || isOtherAgentReply(agentName, entry) {
			return BuildMessagesDefault(agentName, branch, entries[i:])
		}
	}
	return BuildMessagesDefault(agentName, branch, entries)
}

func isOtherAgentReply(currentAgentName string, entry *session.MessageEntry) bool {
	return entry.Author != currentAgentName && entry.Author != session.AuthorUser
}

func ConvertForeignEntry(entry *session.MessageEntry) *session.MessageEntry {
	if !messageForHistory(entry.Message) {
		return entry
	}

	content := []message.Content{&message.TextContent{Text: "For context:"}}
	for _, p := range entry.Message.TextContents() {
		if tc, ok := p.(*message.TextContent); ok {
			content = append(content, &message.TextContent{
				Text: fmt.Sprintf("[%s] said: %s", entry.Author, tc.Text),
			})
		}
	}
	for _, tc := range entry.Message.ToolCalls() {
		content = append(content, &message.TextContent{
			Text: fmt.Sprintf("[%s] called tool %q with parameters: %v", entry.Author, tc.Name, tc.Args),
		})
	}
	for _, tr := range entry.Message.ToolResults() {
		content = append(content, &message.TextContent{
			Text: fmt.Sprintf("[%s] tool returned result: %v", entry.Author, tr.Content),
		})
	}

	converted := shallowCopyEntry(entry)
	converted.Author = "user"
	converted.Message = &message.Message{Role: message.RoleUser, Content: content}
	return converted
}

const requestEUCFunctionCallName = "adk_request_credential"

func isAuthEntry(entry *session.MessageEntry) bool {
	if entry.Message == nil {
		return false
	}
	for _, tc := range entry.Message.ToolCalls() {
		if tc.Name == requestEUCFunctionCallName {
			return true
		}
	}
	return false
}

func messageForHistory(msg *message.Message) bool {
	if msg == nil || msg.Role == "" {
		return false
	}
	return len(msg.TextContents()) > 0 ||
		len(msg.ThinkingContents()) > 0 ||
		len(msg.ToolCalls()) > 0 ||
		len(msg.ToolResults()) > 0
}

// shallowCopyEntry returns a header-only copy of the entry; callers are responsible
// for replacing Message when they need a distinct payload.
func shallowCopyEntry(entry *session.MessageEntry) *session.MessageEntry {
	if entry == nil {
		return nil
	}
	newEntry := *entry
	return &newEntry
}
