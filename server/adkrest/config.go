package adkrest

import (
	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/telemetry"
)

// Config contains the dependencies required by the ADK REST API server.
type Config struct {
	SessionService   session.Service
	AgentLoader      agent.Loader
	TelemetryOptions []telemetry.Option
}
