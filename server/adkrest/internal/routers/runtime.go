package routers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
)

// RuntimeAPIRouter defines the routes for the Runtime API.
type RuntimeAPIRouter struct {
	runtimeController *controllers.RuntimeAPIController
}

// NewRuntimeAPIRouter creates a new RuntimeAPIRouter.
func NewRuntimeAPIRouter(controller *controllers.RuntimeAPIController) *RuntimeAPIRouter {
	return &RuntimeAPIRouter{runtimeController: controller}
}

// Routes returns the routes for the Runtime API.
func (r *RuntimeAPIRouter) Routes() Routes {
	return Routes{
		Route{
			Name:        "RunAgent",
			Methods:     []string{http.MethodPost, http.MethodOptions},
			Pattern:     "/run",
			HandlerFunc: controllers.NewErrorHandler(r.runtimeController.RunHandler),
		},
		Route{
			Name:        "RunAgentSse",
			Methods:     []string{http.MethodPost, http.MethodOptions},
			Pattern:     "/run_sse",
			HandlerFunc: controllers.NewErrorHandler(r.runtimeController.RunSSEHandler),
		},
	}
}
