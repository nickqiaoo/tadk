package universal

import (
	"testing"

	"github.com/nickqiaoo/tadk/cmd/launcher"
)

func TestApplyPresetAndPickKeyword(t *testing.T) {
	cfg := &launcher.Config{}
	args, keyword, err := applyPresetAndPickKeyword(cfg, []string{"server", "--port", "8080"})
	if err != nil {
		t.Fatalf("applyPresetAndPickKeyword(): %v", err)
	}
	if cfg.Preset != launcher.PresetServer {
		t.Fatalf("Preset = %q, want %q", cfg.Preset, launcher.PresetServer)
	}
	if keyword != "web" {
		t.Fatalf("keyword = %q, want web", keyword)
	}
	if len(args) != 2 || args[0] != "--port" || args[1] != "8080" {
		t.Fatalf("args = %#v, want [--port 8080]", args)
	}
}

func TestApplyPresetAndPickKeywordLocalPicksConsole(t *testing.T) {
	cfg := &launcher.Config{}
	_, keyword, err := applyPresetAndPickKeyword(cfg, []string{"local"})
	if err != nil {
		t.Fatalf("applyPresetAndPickKeyword(): %v", err)
	}
	if keyword != "console" {
		t.Fatalf("keyword = %q, want console", keyword)
	}
}
