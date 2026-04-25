package routers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
)

// DebugAPIRouter defines the routes for the Debug API.
type DebugAPIRouter struct {
	runtimeController *controllers.DebugAPIController
}

// NewDebugAPIRouter creates a new DebugAPIRouter.
func NewDebugAPIRouter(controller *controllers.DebugAPIController) *DebugAPIRouter {
	return &DebugAPIRouter{runtimeController: controller}
}

// Routes returns the routes for the Debug API.
func (r *DebugAPIRouter) Routes() Routes {
	return Routes{
		Route{
			Name:        "GetTraceDict",
			Methods:     []string{http.MethodGet},
			Pattern:     "/debug/trace/{event_id}",
			HandlerFunc: r.runtimeController.EventSpanHandler,
		},
		Route{
			Name:        "GetEventGraph",
			Methods:     []string{http.MethodGet},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions/{session_id}/events/{event_id}/graph",
			HandlerFunc: r.runtimeController.EventGraphHandler,
		},

		Route{
			Name:        "GetSessionTrace",
			Methods:     []string{http.MethodGet},
			Pattern:     "/debug/trace/session/{session_id}",
			HandlerFunc: r.runtimeController.SessionSpansHandler,
		},
	}
}
