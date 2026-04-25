package adkrest

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/routers"
	"github.com/nickqiaoo/tadk/server/adkrest/internal/services"
	"github.com/nickqiaoo/tadk/telemetry"
)

// NewHandler creates and returns an http.Handler for the ADK REST API.
func NewHandler(config *Config, sseWriteTimeout time.Duration) http.Handler {
	debugTelemetry := services.NewDebugTelemetry()
	config.TelemetryOptions = append(config.TelemetryOptions, telemetry.WithSpanProcessors(debugTelemetry.SpanProcessor()))
	config.TelemetryOptions = append(config.TelemetryOptions, telemetry.WithLogRecordProcessors(debugTelemetry.LogProcessor()))

	router := mux.NewRouter().StrictSlash(true)
	// TODO: Allow taking a prefix to allow customizing the path
	// where the ADK REST API will be served.
	setupRouter(router,
		routers.NewSessionsAPIRouter(controllers.NewSessionsAPIController(config.SessionService)),
		routers.NewRuntimeAPIRouter(controllers.NewRuntimeAPIController(config.SessionService, config.AgentLoader, sseWriteTimeout)),
		routers.NewAppsAPIRouter(controllers.NewAppsAPIController(config.AgentLoader)),
		routers.NewDebugAPIRouter(controllers.NewDebugAPIController(config.SessionService, config.AgentLoader, debugTelemetry)),
		&routers.EvalAPIRouter{},
	)
	return router
}

func setupRouter(router *mux.Router, subrouters ...routers.Router) *mux.Router {
	routers.SetupSubRouters(router, subrouters...)
	return router
}
