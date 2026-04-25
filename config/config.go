package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultPath is the framework-wide default config file name.
	DefaultPath = "adk.toml"
)

// Config holds framework runtime configuration loaded from adk.toml.
type Config struct {
	Model     ModelConfig               `toml:"model"`
	Providers map[string]ProviderConfig `toml:"providers"`
}

// ModelConfig identifies the default model the application should use.
type ModelConfig struct {
	Provider string `toml:"provider"`
	Name     string `toml:"name"`
}

// ProviderConfig holds connection/auth settings for one model provider.
type ProviderConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
}

// Load reads and validates ./adk.toml from the current working directory.
func Load() (Config, error) {
	return LoadFile(DefaultPath)
}

// LoadFile reads and validates the config file at path.
func LoadFile(path string) (Config, error) {
	var cfg Config
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("config file %q not found in current working directory", path)
		}
		return Config{}, fmt.Errorf("decode %q: %w", path, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return Config{}, fmt.Errorf("unknown config keys in %q: %s", path, strings.Join(keys, ", "))
	}

	cfg.Model.Provider = strings.ToLower(strings.TrimSpace(cfg.Model.Provider))
	cfg.Model.Name = strings.TrimSpace(cfg.Model.Name)
	cfg.Providers = normalizeProviders(cfg.Providers)
	if cfg.Model.Provider == "" {
		return Config{}, fmt.Errorf("%q must set [model].provider", path)
	}
	if cfg.Model.Name == "" {
		return Config{}, fmt.Errorf("%q must set [model].name", path)
	}
	if _, ok := cfg.Provider(cfg.Model.Provider); !ok {
		return Config{}, fmt.Errorf("%q must define [providers.%s]", path, cfg.Model.Provider)
	}

	return cfg, nil
}

func (c Config) Provider(name string) (ProviderConfig, bool) {
	if len(c.Providers) == 0 {
		return ProviderConfig{}, false
	}
	cfg, ok := c.Providers[strings.ToLower(strings.TrimSpace(name))]
	return cfg, ok
}

func normalizeProviders(src map[string]ProviderConfig) map[string]ProviderConfig {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]ProviderConfig, len(src))
	for name, cfg := range src {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" {
			continue
		}
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
		dst[normalizedName] = cfg
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}
