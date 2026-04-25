package runner

import (
	"context"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/session"
)

// eventPersistor converts runtime events into durable session entries.
type eventPersistor struct {
	svc session.Service
}

func newEventPersistor(svc session.Service) *eventPersistor {
	return &eventPersistor{svc: svc}
}

func (p *eventPersistor) appendMessage(ctx context.Context, storedSession session.Session, invCtx agent.InvocationContext, msg *message.Message) error {
	if msg == nil {
		return nil
	}
	entry := session.NewMessageLogEntry(lastEntryID(storedSession), msg, session.AuthorUser, invCtx.InvocationID(), "")
	return p.svc.AppendEntry(ctx, &session.AppendEntryRequest{
		AppName:   storedSession.AppName(),
		UserID:    storedSession.UserID(),
		SessionID: storedSession.ID(),
		Entry:     entry,
	})
}

func (p *eventPersistor) persistEvent(ctx context.Context, storedSession session.Session, ev event.Event) error {
	entry := durableEntryFromEvent(ev)
	if entry != nil {
		if b := entry.Base(); b != nil {
			b.ParentID = lastEntryID(storedSession)
		}
		if err := p.svc.AppendEntry(ctx, &session.AppendEntryRequest{
			AppName:   storedSession.AppName(),
			UserID:    storedSession.UserID(),
			SessionID: storedSession.ID(),
			Entry:     entry,
		}); err != nil {
			return err
		}
	}
	return nil
}

// durableEntryFromEvent materializes a session entry from an event. Invariant:
// agent.WrapRun populates Author/Branch/InvocationID on every event before it
// reaches the runner, so this function does not re-derive them from invCtx.
func durableEntryFromEvent(ev event.Event) session.DurableEntry {
	if ev == nil {
		return nil
	}
	msgEnd, ok := ev.(*event.MessageEnd)
	if !ok || msgEnd == nil || msgEnd.Message == nil {
		return nil
	}
	msg := msgEnd.Message
	// User messages are persisted exclusively by persistUserMessage at
	// invocation start. llmagent re-emits the user message as a MessageEnd
	// for downstream observability; skip it here to avoid double-write.
	if msg.Role == message.RoleUser {
		return nil
	}
	m := ev.Meta()
	entry := session.NewMessageLogEntry("", msg, m.Author, m.InvocationID, m.Branch)
	entry.StopReason = msg.StopReason
	entry.Usage = msg.Usage
	entry.ErrorCode = msg.ErrorCode
	entry.ErrorMessage = msg.ErrorMessage
	return entry
}

func lastEntryID(sess session.Session) string {
	if sess == nil || sess.Entries().Len() == 0 {
		return ""
	}
	if b := sess.Entries().At(sess.Entries().Len() - 1).Base(); b != nil {
		return b.ID
	}
	return ""
}
