package web

import (
	"testing"

	"github.com/gorilla/mux"

	"github.com/nickqiaoo/tadk/cmd/launcher"
)

type stubSublauncher struct {
	keyword string
}

func (s stubSublauncher) Keyword() string { return s.keyword }
func (s stubSublauncher) Parse(args []string) ([]string, error) {
	return args, nil
}
func (s stubSublauncher) CommandLineSyntax() string { return "" }
func (s stubSublauncher) SimpleDescription() string { return "" }
func (s stubSublauncher) SetupSubrouters(router *mux.Router, config *launcher.Config) error {
	return nil
}
func (s stubSublauncher) UserMessage(webURL string, printer func(v ...any)) {}

func TestActivateDefaultSublaunchersForHTTPPreset(t *testing.T) {
	w := &webLauncher{
		sublaunchers: []Sublauncher{stubSublauncher{keyword: "api"}, stubSublauncher{keyword: "webui"}},
	}

	cfg := &launcher.Config{Preset: launcher.PresetLocalHTTP}
	w.activateDefaultSublaunchers(cfg)

	if _, ok := w.activeSublaunchers["api"]; !ok {
		t.Fatal("default active sublaunchers missing api")
	}
	if len(w.activeSublaunchers) != 1 {
		t.Fatalf("activeSublaunchers len = %d, want 1", len(w.activeSublaunchers))
	}
}
