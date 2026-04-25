// Package steering provides an extension that allows external code to inject
// messages into an agent's conversation while it is running.
package steering

import (
	"sync"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/message"
)

// Extension injects messages into a running agent's conversation.
//
// Use [Extension.Steer] to queue messages that are drained after the current
// turn and injected before the next LLM call. Use [Extension.FollowUp] to
// queue messages that are injected only when the agent would otherwise stop.
//
// The extension is safe for concurrent use; Steer and FollowUp may be called
// from any goroutine while the agent is running.
type Extension struct {
	agent.DefaultExtension

	mu       sync.Mutex
	pending  []*message.Message
	followUp []*message.Message
}

// New creates a new steering extension.
func New() *Extension {
	return &Extension{}
}

// Name returns the extension's identifier.
func (e *Extension) Name() string { return "steering" }

// Steer queues messages to be injected after the current turn completes.
// Thread-safe.
func (e *Extension) Steer(msgs ...*message.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, msgs...)
}

// FollowUp queues messages to be injected when the agent would otherwise stop.
// Unlike Steer, follow-up messages also force another turn via
// [agent.TurnControl.Continue].
// Thread-safe.
func (e *Extension) FollowUp(msgs ...*message.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.followUp = append(e.followUp, msgs...)
}

// AfterTurn drains the steering queue into ctrl. Steering messages take
// priority over follow-up messages; follow-up messages are only returned
// when no steering messages are pending.
func (e *Extension) AfterTurn(ctx agent.InvocationContext, ctrl *agent.TurnControl) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.pending) > 0 {
		ctrl.Messages = append(ctrl.Messages, e.pending...)
		e.pending = nil
		return nil
	}

	if len(e.followUp) > 0 {
		ctrl.Messages = append(ctrl.Messages, e.followUp...)
		ctrl.Continue = true
		e.followUp = nil
	}

	return nil
}
