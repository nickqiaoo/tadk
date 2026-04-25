// Package runner provides a runtime for ADK agents.
package runner

import (
	"context"
	"fmt"
	"iter"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner/parentmap"
	"github.com/nickqiaoo/tadk/session"
)

// Config is used to create a [Runner].
type Config struct {
	AppName string
	// Root agent which starts the execution.
	Agent          agent.Agent
	SessionService session.Service

	// optional
	Extensions []agent.Extension
}

// New creates a new [Runner].
func New(cfg Config) (*Runner, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("root agent is required")
	}

	if cfg.SessionService == nil {
		return nil, fmt.Errorf("session service is required")
	}

	tree, err := parentmap.New(cfg.Agent)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent tree: %w", err)
	}

	return &Runner{
		appName:           cfg.AppName,
		rootAgent:         cfg.Agent,
		sessionService:    cfg.SessionService,
		extensions:        cfg.Extensions,
		tree:              tree,
		persistor:         newEventPersistor(cfg.SessionService),
		checkpointManager: newCheckpointManager(cfg.SessionService, cfg.AppName),
	}, nil
}

// Runner manages the execution of the agent within a session, handling message
// processing, event generation, and interaction with various services like
// session management and memory.
type Runner struct {
	appName           string
	rootAgent         agent.Agent
	sessionService    session.Service
	extensions        []agent.Extension
	tree              *parentmap.Tree
	persistor         *eventPersistor
	checkpointManager *checkpointManager
}

// runOutputs runs the agent for the given user input, yielding agent events.
// The orchestration is delegated to invocation methods; this function only
// wires up the iterator and drives the transfer loop.
func (r *Runner) runOutputs(ctx context.Context, userID, sessionID string, msg *message.Message, opts agent.RunOptions) iter.Seq2[event.Event, error] {
	return func(yield func(event.Event, error) bool) {
		inv, agentToRun, err := r.newInvocation(ctx, userID, sessionID, msg, opts)
		if err != nil {
			yield(nil, err)
			return
		}
		if err := inv.persistUserMessage(msg); err != nil {
			yield(nil, err)
			return
		}

		for agentToRun != nil {
			next, stop, err := inv.runAgent(agentToRun, yield)
			if err != nil {
				yield(nil, err)
				return
			}
			if stop {
				return
			}
			agentToRun = next
		}

		if err := inv.finalize(); err != nil {
			yield(nil, err)
		}
	}
}

