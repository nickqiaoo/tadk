package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	SelectedProvider string            `json:"selected_provider,omitempty"`
	SelectedModel    string            `json:"selected_model,omitempty"`
	Theme            string            `json:"theme,omitempty"`
	ProviderModels   map[string]string `json:"provider_models,omitempty"`
}

func LoadSettings(path string) (Settings, error) {
	var settings Settings
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return Settings{}, err
	}
	if len(data) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}
	if len(settings.ProviderModels) == 0 {
		settings.ProviderModels = nil
	}
	settings.SelectedProvider = strings.TrimSpace(strings.ToLower(settings.SelectedProvider))
	settings.SelectedModel = strings.TrimSpace(settings.SelectedModel)
	return settings, nil
}

func SaveSettings(path string, settings Settings) error {
	if settings.ProviderModels != nil && len(settings.ProviderModels) == 0 {
		settings.ProviderModels = nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Settings) RememberModel(provider, model string) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return
	}
	s.SelectedProvider = provider
	s.SelectedModel = model
	if s.ProviderModels == nil {
		s.ProviderModels = make(map[string]string)
	}
	s.ProviderModels[provider] = model
}

func (s Settings) ModelForProvider(provider string) string {
	if len(s.ProviderModels) == 0 {
		return ""
	}
	return strings.TrimSpace(s.ProviderModels[strings.TrimSpace(strings.ToLower(provider))])
}
