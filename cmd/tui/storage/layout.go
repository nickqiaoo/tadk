package storage

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	AppName = "tadk_tui"
)

func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tadk"), nil
}

func SessionsDir() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sessions"), nil
}

func SettingsPath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "settings.json"), nil
}

func DefaultUserID() string {
	u, err := user.Current()
	if err != nil {
		return "local"
	}
	name := strings.TrimSpace(u.Username)
	if name == "" {
		return "local"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}
