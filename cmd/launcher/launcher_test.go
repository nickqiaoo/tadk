package launcher

import (
	"testing"

	filesession "github.com/nickqiaoo/tadk/session/file"
)

func TestApplyPreset(t *testing.T) {
	var cfg Config
	if err := cfg.ApplyPreset(PresetServer); err != nil {
		t.Fatalf("ApplyPreset(): %v", err)
	}
	if cfg.Preset != PresetServer {
		t.Fatalf("Preset = %q, want %q", cfg.Preset, PresetServer)
	}
}

func TestApplyPresetRejectsUnknown(t *testing.T) {
	var cfg Config
	if err := cfg.ApplyPreset("nonsense"); err == nil {
		t.Fatal("ApplyPreset(\"nonsense\") succeeded, want error")
	}
}

func TestValidateModesRejectsFileStoreForServerPreset(t *testing.T) {
	svc, err := filesession.NewService(filesession.ServiceConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := svc.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	cfg := &Config{SessionService: svc}
	if err := cfg.ApplyPreset(PresetServer); err != nil {
		t.Fatalf("ApplyPreset(): %v", err)
	}
	if err := cfg.ValidateModes(); err == nil {
		t.Fatal("ValidateModes() succeeded, want file store rejected for server preset")
	}
}

func TestValidateModesAllowsFileStoreForLocalHTTP(t *testing.T) {
	svc, err := filesession.NewService(filesession.ServiceConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := svc.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	cfg := &Config{SessionService: svc, Preset: PresetLocalHTTP}
	if err := cfg.ValidateModes(); err != nil {
		t.Fatalf("ValidateModes(): %v", err)
	}
}
