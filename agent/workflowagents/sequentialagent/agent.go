// Package sequentialagent provides an agent that runs its sub-agents in a sequence.
package sequentialagent

import (
	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/workflowagents/loopagent"
)

// New creates a SequentialAgent.
//
// SequentialAgent executes its sub-agents once, in the order they are listed.
//
// Use the SequentialAgent when you want the execution to occur in a fixed,
// strict order.
func New(cfg Config) (agent.Agent, error) {
	baseAgent, err := loopagent.New(loopagent.Config{
		AgentConfig:   cfg.AgentConfig,
		MaxIterations: 1,
	})
	if err != nil {
		return nil, err
	}

	return &sequentialAgent{Agent: baseAgent}, nil
}

// Config defines the configuration for a SequentialAgent.
type Config struct {
	// Basic agent setup.
	AgentConfig agent.Config
}

type sequentialAgent struct {
	agent.Agent
}

func (a *sequentialAgent) AgentType() agent.Type {
	return agent.TypeSequentialAgent
}
