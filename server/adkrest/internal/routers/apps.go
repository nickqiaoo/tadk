package routers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
)

// AppsAPIRouter defines the routes for the Apps API.
type AppsAPIRouter struct {
	appsController *controllers.AppsAPIController
}

// NewAppsAPIRouter creates a new AppsAPIRouter.
func NewAppsAPIRouter(controller *controllers.AppsAPIController) *AppsAPIRouter {
	return &AppsAPIRouter{appsController: controller}
}

// Routes returns the routes for the Apps API.
func (r *AppsAPIRouter) Routes() Routes {
	return Routes{
		Route{
			Name:        "ListApps",
			Methods:     []string{http.MethodGet},
			Pattern:     "/list-apps",
			HandlerFunc: r.appsController.ListAppsHandler,
		},
	}
}
