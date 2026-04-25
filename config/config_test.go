package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	t.Run("loads config and normalizes provider names", func(t *testing.T) {
		path := writeConfigFile(t, `
[model]
provider = " Google "
name = " gemini-3-flash-preview "

[providers.google]
api_key = " test-key "
`)

		cfg, err := LoadFile(path)
		if err != nil {
			t.Fatalf("LoadFile() unexpected error: %v", err)
		}

		if cfg.Model.Provider != "google" {
			t.Fatalf("Model.Provider = %q, want %q", cfg.Model.Provider, "google")
		}
		if cfg.Model.Name != "gemini-3-flash-preview" {
			t.Fatalf("Model.Name = %q, want %q", cfg.Model.Name, "gemini-3-flash-preview")
		}
		googleCfg, ok := cfg.Provider("google")
		if !ok {
			t.Fatal("Provider(google) = missing, want config")
		}
		if googleCfg.APIKey != "test-key" {
			t.Fatalf("Provider(google).APIKey = %q, want %q", googleCfg.APIKey, "test-key")
		}
	})

	t.Run("rejects unknown keys", func(t *testing.T) {
		path := writeConfigFile(t, `
[model]
provider = "google"
name = "gemini-3-flash-preview"

[providers.google]
api_key = "test-key"
extra = "nope"
`)

		_, err := LoadFile(path)
		if err == nil {
			t.Fatal("LoadFile() error = nil, want unknown key error")
		}
		if !strings.Contains(err.Error(), "unknown config keys") {
			t.Fatalf("LoadFile() error = %v, want unknown config keys", err)
		}
	})

	t.Run("requires model provider", func(t *testing.T) {
		path := writeConfigFile(t, `
[model]
name = "custom-model"

[providers.google]
api_key = "test-key"
`)

		_, err := LoadFile(path)
		if err == nil {
			t.Fatal("LoadFile() error = nil, want required key error")
		}
		if !strings.Contains(err.Error(), "[model].provider") {
			t.Fatalf("LoadFile() error = %v, want [model].provider", err)
		}
	})

	t.Run("requires provider block for selected model", func(t *testing.T) {
		path := writeConfigFile(t, `
[model]
provider = "google"
name = "custom-model"
`)

		_, err := LoadFile(path)
		if err == nil {
			t.Fatal("LoadFile() error = nil, want missing provider block error")
		}
		if !strings.Contains(err.Error(), "[providers.google]") {
			t.Fatalf("LoadFile() error = %v, want [providers.google]", err)
		}
	})

	t.Run("reports missing file", func(t *testing.T) {
		_, err := LoadFile(filepath.Join(t.TempDir(), "missing.toml"))
		if err == nil {
			t.Fatal("LoadFile() error = nil, want missing file error")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("LoadFile() error = %v, want not found", err)
		}
	})
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "adk.toml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}
	return path
}
