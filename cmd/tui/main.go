package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nickqiaoo/tadk/cmd/tui/app"
	"github.com/nickqiaoo/tadk/cmd/tui/runtime"
	"github.com/nickqiaoo/tadk/cmd/tui/storage"
	"github.com/nickqiaoo/tadk/cmd/tui/tools"
)

func main() {
	ctx := context.Background()

	var (
		configPath   string
		sessionsDir  string
		settingsPath string
		resumeID     string
	)

	flag.StringVar(&configPath, "config", "", "Path to adk.toml. Defaults to upward lookup from cwd.")
	flag.StringVar(&sessionsDir, "sessions-dir", "", "Directory for file-based sessions. Defaults to ~/.tadk/sessions.")
	flag.StringVar(&settingsPath, "settings", "", "Path to settings.json. Defaults to ~/.tadk/settings.json.")
	flag.StringVar(&resumeID, "resume", "", "Resume an existing session ID.")
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if configPath == "" {
		configPath, err = runtime.FindConfig(cwd)
		if err != nil {
			log.Fatal(err)
		}
	}
	if sessionsDir == "" {
		sessionsDir, err = storage.SessionsDir()
		if err != nil {
			log.Fatal(err)
		}
	}
	if settingsPath == "" {
		settingsPath, err = storage.SettingsPath()
		if err != nil {
			log.Fatal(err)
		}
	}

	settings, err := storage.LoadSettings(settingsPath)
	if err != nil {
		log.Fatal(err)
	}

	factory, err := runtime.NewFactory(configPath, tools.Registry())
	if err != nil {
		log.Fatal(err)
	}

	sessionManager, err := runtime.NewSessionManager(storage.AppName, storage.DefaultUserID(), sessionsDir)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sessionManager.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close session manager: %v\n", err)
		}
	}()

	model, err := app.New(app.Deps{
		Context:      ctx,
		Factory:      factory,
		Sessions:     sessionManager,
		Settings:     settings,
		SettingsPath: settingsPath,
		SessionID:    resumeID,
	})
	if err != nil {
		log.Fatal(err)
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.SetProgram(program)
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
