package routers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/server/adkrest/controllers"
)

// EvalAPIRouter defines the routes for the Eval API.
type EvalAPIRouter struct{}

// Routes returns the routes for the Apps API.
func (r *EvalAPIRouter) Routes() Routes {
	return Routes{
		Route{
			Name:        "ListEvalSets",
			Methods:     []string{http.MethodGet},
			Pattern:     "/apps/{app_name}/eval_sets",
			HandlerFunc: controllers.Unimplemented,
		},
		Route{
			Name:        "ListEvalSets",
			Methods:     []string{http.MethodPost, http.MethodOptions},
			Pattern:     "/apps/{app_name}/eval_sets/{eval_set_name}",
			HandlerFunc: controllers.Unimplemented,
		},
		Route{
			Name:        "ListEvalResults",
			Methods:     []string{http.MethodGet},
			Pattern:     "/apps/{app_name}/eval_results",
			HandlerFunc: controllers.Unimplemented,
		},
	}
}
