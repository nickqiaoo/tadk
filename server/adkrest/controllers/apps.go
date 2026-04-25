package controllers

import (
	"net/http"

	"github.com/nickqiaoo/tadk/agent"
)

// AppsAPIController is the controller for the Apps API.
type AppsAPIController struct {
	agentLoader agent.Loader
}

// NewAppsAPIController creates a controller for Apps API.
func NewAppsAPIController(agentLoader agent.Loader) *AppsAPIController {
	return &AppsAPIController{agentLoader: agentLoader}
}

// ListAppsHandler handles listing all loaded agents.
func (c *AppsAPIController) ListAppsHandler(rw http.ResponseWriter, req *http.Request) {
	apps := c.agentLoader.ListAgents()
	EncodeJSONResponse(apps, http.StatusOK, rw)
}
