// Package routers defines the HTTP routes for the ADK REST API.
package routers

import (
	"net/http"

	"github.com/gorilla/mux"
)

// A Route defines the parameters for an api endpoint
type Route struct {
	Name        string
	Methods     []string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

// Routes is a list of defined api endpoints
type Routes []Route

// Router defines the required methods for retrieving api routes
type Router interface {
	Routes() Routes
}

// NewRouter creates a new router for any number of api routers
func NewRouter(routers ...Router) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	SetupSubRouters(router)
	return router
}

// SetupSubRouters adds routes from subrouter to the naub router
func SetupSubRouters(router *mux.Router, subrouters ...Router) {
	for _, api := range subrouters {
		for _, route := range api.Routes() {
			var handler http.Handler = route.HandlerFunc

			router.
				Methods(route.Methods...).
				Path(route.Pattern).
				Name(route.Name).
				Handler(handler)
		}
	}
}
