package runner

import (
	"context"
	"iter"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
)

// Run executes a single runner invocation and returns the raw agent event
// stream. The terminal AgentEnd event carries the messages appended during the
// run (as pointers into the emitted MessageEnd stream) and the terminal error,
// if any. AgentEnd is always emitted unless the consumer aborts iteration.
//
// The AgentEnd payload is a projection of MessageEnd events for consumer
// convenience. It is not persisted separately — session durability still flows
// through MessageEnd in the persistor.
func (r *Runner) Run(ctx context.Context, userID, sessionID string, msg *message.Message, cfg agent.RunOptions) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		var (
			messages   []*message.Message
			sawUserEnd bool
			runErr     error
			aborted    bool
		)
		defer func() {
			if aborted {
				return
			}
			out := messages
			if msg != nil && !sawUserEnd {
				out = append([]*message.Message{msg}, messages...)
			}
			// AgentEnd bookends AgentStart; emit unconditionally on any
			// exit path that wasn't an explicit consumer abort so observers
			// see a symmetric stream even on errors.
			yield(event.NewAgentEnd(out, runErr), nil)
		}()

		if !yield(event.NewAgentStart(), nil) {
			aborted = true
			return
		}
		for ev, err := range r.runOutputs(ctx, userID, sessionID, msg, cfg) {
			if err != nil {
				runErr = err
				if !yield(nil, err) {
					aborted = true
				}
				return
			}
			if ev == nil {
				continue
			}
			if msgEnd, ok := ev.(*event.MessageEnd); ok && msgEnd != nil {
				if msgEnd.Message.Role == message.RoleUser {
					sawUserEnd = true
				}
				if msgEnd.Message != nil {
					messages = append(messages, msgEnd.Message)
				}
			}
			if !yield(ev, nil) {
				aborted = true
				return
			}
		}
	}
}
