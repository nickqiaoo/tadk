package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/resume"
	"github.com/nickqiaoo/tadk/session"
)

// checkpointManager handles loading, saving, and clearing checkpoints.
type checkpointManager struct {
	svc     session.Service
	appName string
}

func newCheckpointManager(svc session.Service, appName string) *checkpointManager {
	return &checkpointManager{svc: svc, appName: appName}
}

func (c *checkpointManager) load(ctx context.Context, storedSession session.Session) (*session.Checkpoint, error) {
	if storedSession == nil {
		return nil, nil
	}
	checkpoint, err := c.svc.GetCheckpoint(ctx, &session.CheckpointRequest{
		AppName:   c.appName,
		UserID:    storedSession.UserID(),
		SessionID: storedSession.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (c *checkpointManager) save(ctx context.Context, storedSession session.Session, invCtx agent.InvocationContext, interrupt *resume.InterruptData, entry session.DurableEntry) error {
	if storedSession == nil || interrupt == nil {
		return nil
	}

	contexts := make(map[string]*resumeContext, len(interrupt.Contexts))
	for _, interruptCtx := range interrupt.Contexts {
		for cur := interruptCtx; cur != nil; cur = cur.Parent {
			contexts[cur.ID] = &resumeContext{
				address: cur.Address,
				info:    cur.Info,
			}
		}
	}

	pending := make(map[string]*session.PendingInterrupt)
	for address, interruptID := range interrupt.Address2ID {
		agentAddress, toolCallID := splitToolAddress(address)
		interruptCtx := contexts[interruptID]
		toolCall := toolCallForAddress(storedSession.Entries(), agentAddress, toolCallID)
		pending[interruptID] = &session.PendingInterrupt{
			ID:           interruptID,
			Address:      address,
			AgentAddress: agentAddress,
			ToolCallID:   toolCallID,
			ToolName:     toolCall.name,
			ToolArgs:     toolCall.args,
			State:        interrupt.ID2State[interruptID],
		}
		if interruptCtx != nil {
			pending[interruptID].Info = interruptCtx.info
		}
	}

	var workflowID string
	existing, err := c.svc.GetCheckpoint(ctx, &session.CheckpointRequest{
		AppName:   c.appName,
		UserID:    storedSession.UserID(),
		SessionID: storedSession.ID(),
	})
	if err != nil {
		return fmt.Errorf("failed to load existing checkpoint: %w", err)
	}
	if existing != nil {
		workflowID = existing.WorkflowID
	}

	entryID := ""
	if entry != nil && entry.Base() != nil {
		entryID = entry.Base().ID
	}
	checkpoint := &session.Checkpoint{
		Version:           1,
		Status:            session.CheckpointStatusInterrupted,
		Branch:            invCtx.Branch(),
		InvocationID:      invCtx.InvocationID(),
		EntryID:           entryID,
		WorkflowID:        workflowID,
		PendingInterrupts: pending,
	}
	return c.svc.SaveCheckpoint(ctx, &session.SaveCheckpointRequest{
		CheckpointRequest: session.CheckpointRequest{
			AppName:   c.appName,
			UserID:    storedSession.UserID(),
			SessionID: storedSession.ID(),
		},
		Checkpoint: checkpoint,
	})
}

func (c *checkpointManager) clear(ctx context.Context, storedSession session.Session) error {
	if storedSession == nil {
		return nil
	}
	return c.svc.ClearCheckpoint(ctx, &session.CheckpointRequest{
		AppName:   c.appName,
		UserID:    storedSession.UserID(),
		SessionID: storedSession.ID(),
	})
}

type resumeContext struct {
	address string
	info    any
}

type checkpointToolCall struct {
	name string
	args map[string]any
}

func toolCallForAddress(entries session.Entries, agentAddress, toolCallID string) checkpointToolCall {
	var fallback checkpointToolCall
	for i := entries.Len() - 1; i >= 0; i-- {
		entry := entries.At(i)
		msgEntry, ok := entry.(*session.MessageEntry)
		if !ok || msgEntry == nil || msgEntry.Message == nil {
			continue
		}
		for _, toolCall := range msgEntry.Message.ToolCalls() {
			if toolCall.ID != toolCallID {
				continue
			}
			candidate := checkpointToolCall{
				name: toolCall.Name,
				args: toolCall.Args,
			}
			if fallback.name == "" {
				fallback = candidate
			}
			if toolAddressMatchesAuthor(agentAddress, msgEntry.Author) {
				return candidate
			}
		}
	}
	return fallback
}

func toolAddressMatchesAuthor(agentAddress, author string) bool {
	if agentAddress == "" || author == "" {
		return true
	}
	if agentAddress == author {
		return true
	}
	return strings.HasSuffix(agentAddress, "."+author)
}

func splitToolAddress(address string) (agentAddress, toolCallID string) {
	const marker = ":tool:"
	if idx := strings.LastIndex(address, marker); idx >= 0 {
		return address[:idx], address[idx+len(marker):]
	}
	const prefix = "tool:"
	if strings.HasPrefix(address, prefix) {
		return "", strings.TrimPrefix(address, prefix)
	}
	return address, ""
}
