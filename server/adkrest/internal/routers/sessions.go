package routers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
)

// SessionsAPIRouter defines the routes for the Sessions API.
type SessionsAPIRouter struct {
	sessionController *controllers.SessionsAPIController
}

// NewSessionsAPIRouter creates a new SessionsAPIRouter.
func NewSessionsAPIRouter(controller *controllers.SessionsAPIController) *SessionsAPIRouter {
	return &SessionsAPIRouter{sessionController: controller}
}

// Routes returns the routes for the Sessions API.
func (r *SessionsAPIRouter) Routes() Routes {
	return Routes{
		Route{
			Name:        "GetSession",
			Methods:     []string{http.MethodGet},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions/{session_id}",
			HandlerFunc: r.sessionController.GetSessionHandler,
		},
		Route{
			Name:        "CreateSession",
			Methods:     []string{http.MethodPost},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions",
			HandlerFunc: r.sessionController.CreateSessionHandler,
		},
		Route{
			Name:        "CreateSessionWithId",
			Methods:     []string{http.MethodPost},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions/{session_id}",
			HandlerFunc: r.sessionController.CreateSessionHandler,
		},
		Route{
			Name:        "DeleteSession",
			Methods:     []string{http.MethodDelete, http.MethodOptions},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions/{session_id}",
			HandlerFunc: r.sessionController.DeleteSessionHandler,
		},
		Route{
			Name:        "ListSessions",
			Methods:     []string{http.MethodGet},
			Pattern:     "/apps/{app_name}/users/{user_id}/sessions",
			HandlerFunc: r.sessionController.ListSessionsHandler,
		},
	}
}
