// Package api provides a sublauncher that adds ADK REST API capabilities.
package api

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/nickqiaoo/tadk/cmd/launcher"
	weblauncher "github.com/nickqiaoo/tadk/cmd/launcher/web"
	"github.com/nickqiaoo/tadk/internal/cli/util"
	"github.com/nickqiaoo/tadk/server/adkrest"
)

// apiConfig contains parametres for lauching ADK REST API
type apiConfig struct {
	frontendAddress string
	sseWriteTimeout time.Duration
}

// apiLauncher can launch ADK REST API
type apiLauncher struct {
	flags  *flag.FlagSet
	config *apiConfig
}

// CommandLineSyntax returns the command-line syntax for the API launcher.
func (a *apiLauncher) CommandLineSyntax() string {
	return util.FormatFlagUsage(a.flags)
}

// Adds CORS headers which allow calling ADK REST API from another web app (like ADK WebUI)
func corsWithArgs(frontendAddress string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", frontendAddress)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserMessage implements web.Sublauncher. Prints message to the user
func (a *apiLauncher) UserMessage(webURL string, printer func(v ...any)) {
	printer(fmt.Sprintf("       api:  you can access API using %s/api", webURL))
	printer(fmt.Sprintf("       api:      for instance: %s/api/list-apps", webURL))
}

// SetupSubrouters adds the API router to the parent router.
func (a *apiLauncher) SetupSubrouters(router *mux.Router, config *launcher.Config) error {
	// Create the ADK REST API handler
	apiHandler := adkrest.NewHandler(&adkrest.Config{
		SessionService:   config.SessionService,
		AgentLoader:      config.AgentLoader,
		TelemetryOptions: config.TelemetryOptions,
	}, a.config.sseWriteTimeout)

	// Wrap it with CORS middleware
	corsHandler := corsWithArgs(a.config.frontendAddress)(apiHandler)

	// Register it at the /api/ path
	router.Methods("GET", "POST", "DELETE", "OPTIONS").PathPrefix("/api/").Handler(
		http.StripPrefix("/api", corsHandler),
	)

	return nil
}

// Keyword implements web.Sublauncher. Returns the command-line keyword for API launcher.
func (a *apiLauncher) Keyword() string {
	return "api"
}

// Parse parses the command-line arguments for the API launcher.
func (a *apiLauncher) Parse(args []string) ([]string, error) {
	err := a.flags.Parse(args)
	if err != nil || !a.flags.Parsed() {
		return nil, fmt.Errorf("failed to parse api flags: %v", err)
	}
	restArgs := a.flags.Args()
	return restArgs, nil
}

// SimpleDescription implements web.Sublauncher. Returns a simple description of the API launcher.
func (a *apiLauncher) SimpleDescription() string {
	return "starts ADK REST API server, accepting origins specified by webui_address (CORS)"
}

// NewLauncher creates new api launcher. It extends Web launcher
func NewLauncher() weblauncher.Sublauncher {
	config := &apiConfig{}

	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.StringVar(&config.frontendAddress, "webui_address", "localhost:8080", "ADK WebUI address as seen from the user browser. It's used to allow CORS requests. Please specify only hostname and (optionally) port.")
	fs.DurationVar(&config.sseWriteTimeout, "sse-write-timeout", 120*time.Second, "SSE server write timeout (i.e. '10s', '2m' - see time.ParseDuration for details) - for writing the SSE response after reading the headers & body")

	return &apiLauncher{
		config: config,
		flags:  fs,
	}
}
