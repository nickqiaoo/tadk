// Package console provides a simple way to interact with an agent from console application.
package console

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/cmd/launcher"
	"github.com/nickqiaoo/tadk/cmd/launcher/internal/telemetry"
	"github.com/nickqiaoo/tadk/cmd/launcher/universal"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/cli/util"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
)

// consoleConfig contains command-line params for console launcher
type consoleConfig struct {
	otelToCloud     bool
	shutdownTimeout time.Duration
}

// consoleLauncher allows to interact with an agent in console
type consoleLauncher struct {
	flags  *flag.FlagSet
	config *consoleConfig
}

// NewLauncher creates new console launcher
func NewLauncher() launcher.SubLauncher {
	config := &consoleConfig{}

	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	fs.DurationVar(&config.shutdownTimeout, "shutdown-timeout", 2*time.Second, "Console shutdown timeout (i.e. '10s', '2m' - see time.ParseDuration for details)")
	fs.BoolVar(&config.otelToCloud, "otel_to_cloud", false, "Enables/disables OpenTelemetry export to GCP: telemetry.googleapis.com.")
	return &consoleLauncher{config: config, flags: fs}
}

// Run implements launcher.SubLauncher. It starts the console interaction loop.
func (l *consoleLauncher) Run(ctx context.Context, config *launcher.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	otel, err := telemetry.InitAndSetGlobalOtelProviders(ctx, config, l.config.otelToCloud)
	if err != nil {
		return fmt.Errorf("telemetry initialization failed: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), l.config.shutdownTimeout)
		defer cancel()
		if err := otel.Shutdown(shutdownCtx); err != nil {
			log.Printf("telemetry shutdown failed: %v", err)
		}
	}()

	sessionService := config.SessionService
	if sessionService == nil {
		sessionService = session.InMemoryService()
	}
	config.SessionService = sessionService
	if err := config.ValidateModes(); err != nil {
		return err
	}

	rootAgent := config.AgentLoader.RootAgent()

	agentRunner, err := runner.New(runner.Config{
		AppName:        "console_app",
		Agent:          rootAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return fmt.Errorf("failed to create runner: %v", err)
	}

	return runConsole(ctx, agentRunner, sessionService)
}

func runConsole(ctx context.Context, agentRunner *runner.Runner, sessionService session.Service) error {
	userID := "cli_user"
	resp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "console_app",
		UserID:  userID,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nUser -> ")
	for {
		userInput, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			fmt.Println("\nEOF detected, exiting...")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Print("\nAgent -> ")
		events := agentRunner.Run(ctx, userID, resp.Session.ID(), message.NewUserMessage(userInput), agent.RunOptions{
			StreamingMode: agent.StreamingModeSSE,
		})
		streamedText := ""
		for ev, err := range events {
			if err != nil {
				return err
			}
			if ev == nil {
				continue
			}
			switch typed := ev.(type) {
			case *event.MessageStart:
				if msg := typed.Message; msg != nil && msg.Role == message.RoleAssistant {
					streamedText = ""
				}
			case *event.TextDelta:
				fmt.Print(typed.Delta)
				streamedText += typed.Delta
			case *event.MessageEnd:
				if msg := typed.Message; msg != nil && msg.Role == message.RoleAssistant {
					if text := msg.Text(); text != "" && text != streamedText {
						fmt.Print(text)
					}
				}
			case *event.AgentEnd:
				if typed.Err != nil {
					return typed.Err
				}
			}
		}
		fmt.Print("\nUser -> ")
	}
}



// Parse implements launcher.SubLauncher.
func (l *consoleLauncher) Parse(args []string) ([]string, error) {
	err := l.flags.Parse(args)
	if err != nil || !l.flags.Parsed() {
		return nil, fmt.Errorf("failed to parse flags: %v", err)
	}
	return l.flags.Args(), nil
}

// Keyword implements launcher.SubLauncher.
func (l *consoleLauncher) Keyword() string {
	return "console"
}

// CommandLineSyntax implements launcher.SubLauncher.
func (l *consoleLauncher) CommandLineSyntax() string {
	return util.FormatFlagUsage(l.flags)
}

// SimpleDescription implements launcher.SubLauncher.
func (l *consoleLauncher) SimpleDescription() string {
	return "runs an agent in console mode."
}

// Execute implements launcher.Launcher.
func (l *consoleLauncher) Execute(ctx context.Context, config *launcher.Config, args []string) error {
	remainingArgs, err := l.Parse(args)
	if err != nil {
		return fmt.Errorf("cannot parse args: %w", err)
	}
	err = universal.ErrorOnUnparsedArgs(remainingArgs)
	if err != nil {
		return fmt.Errorf("cannot parse all the arguments: %w", err)
	}
	return l.Run(ctx, config)
}
