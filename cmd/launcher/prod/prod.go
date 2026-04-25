// Package prod provides easy way to play with ADK with all available options without
// development support (no console, no ADK Web UI) including only production
// options like the REST API and A2A support.
package prod

import (
	"github.com/nickqiaoo/tadk/cmd/launcher"
	"github.com/nickqiaoo/tadk/cmd/launcher/universal"
	"github.com/nickqiaoo/tadk/cmd/launcher/web"
	"github.com/nickqiaoo/tadk/cmd/launcher/web/api"
)

// NewLauncher returns a launcher capable of serving ADK REST API.
func NewLauncher() launcher.Launcher {
	return universal.NewLauncher(web.NewLauncher(api.NewLauncher()))
}
