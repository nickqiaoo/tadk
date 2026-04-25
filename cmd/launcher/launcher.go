// Package launcher provides ways to interact with agents.
package launcher

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/telemetry"
)

// Launcher is the main interface for running an ADK application.
// It is responsible for parsing command-line arguments and executing the
// corresponding logic.
type Launcher interface {
	// Execute parses command-line arguments and runs the launcher.
	Execute(ctx context.Context, config *Config, args []string) error
	// CommandLineSyntax returns a string describing the command-line flags and arguments.
	CommandLineSyntax() string
}

// SubLauncher is an interface for launchers that can be composed within a parent
// launcher, like the universal launcher. Each SubLauncher corresponds to a
// specific mode of operation (e.g., 'console' or 'web').
type SubLauncher interface {
	// Keyword returns the command-line keyword that activates this sub-launcher.
	Keyword() string
	// Parse parses the arguments for the sub-launcher. It should return any unparsed arguments.
	Parse(args []string) ([]string, error)
	// CommandLineSyntax returns a string describing the command-line flags and arguments for the sub-launcher.
	CommandLineSyntax() string
	// SimpleDescription provides a brief, one-line description of the sub-launcher's function.
	SimpleDescription() string
	// Run executes the sub-launcher's main logic.
	Run(ctx context.Context, config *Config) error
}

// Config contains parameters for web & console execution: sessions, agents etc
type Config struct {
	SessionService   session.Service
	AgentLoader      agent.Loader
	A2AOptions       []a2asrv.RequestHandlerOption
	TelemetryOptions []telemetry.Option
	Preset           Preset
}

// Preset names a storage/interface combination. The interface (in-process vs
// HTTP) is determined by which sub-launcher (console vs web) runs, so the
// preset's only real job is to pin session-storage expectations.
type Preset string

const (
	PresetLocal     Preset = "local"      // console, any storage
	PresetLocalHTTP Preset = "local-http" // web, any storage
	PresetServer    Preset = "server"     // web, requires durable (non-file) storage
)

// ApplyPreset validates and records the preset on the config.
func (c *Config) ApplyPreset(preset Preset) error {
	if c == nil {
		return nil
	}
	switch preset {
	case PresetLocal, PresetLocalHTTP, PresetServer:
		c.Preset = preset
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("unknown launcher preset %q", preset)
	}
}

// ValidateModes enforces storage constraints for the chosen preset. Only
// PresetServer imposes a constraint: file-backed sessions are not production
// safe, so reject them and point to PresetLocalHTTP for local file use.
func (c *Config) ValidateModes() error {
	if c == nil || c.SessionService == nil {
		return nil
	}
	if c.Preset == PresetServer && session.KindOf(c.SessionService) == session.StoreKindFile {
		return fmt.Errorf("file session storage is not supported with preset=%q; use preset=%q for local HTTP file storage", PresetServer, PresetLocalHTTP)
	}
	return nil
}
