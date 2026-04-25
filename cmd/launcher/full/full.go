// Package full provides easy way to play with ADK with all available options
package full

import (
	"github.com/nickqiaoo/tadk/cmd/launcher"
	"github.com/nickqiaoo/tadk/cmd/launcher/console"
	"github.com/nickqiaoo/tadk/cmd/launcher/universal"
	"github.com/nickqiaoo/tadk/cmd/launcher/web"
	"github.com/nickqiaoo/tadk/cmd/launcher/web/api"
	"github.com/nickqiaoo/tadk/cmd/launcher/web/webui"
)

// NewLauncher returnes the most versatile universal launcher with all options built-in.
func NewLauncher() launcher.Launcher {
	return universal.NewLauncher(console.NewLauncher(), web.NewLauncher(api.NewLauncher(), webui.NewLauncher()))
}
