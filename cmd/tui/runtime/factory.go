package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/config"
	"github.com/nickqiaoo/tadk/model"
	anthropicadapter "github.com/nickqiaoo/tadk/model/anthropic"
	geminiadapter "github.com/nickqiaoo/tadk/model/gemini"
	openaiadapter "github.com/nickqiaoo/tadk/model/openai"
	"github.com/nickqiaoo/tadk/runner"
	"github.com/nickqiaoo/tadk/session"
	adktool "github.com/nickqiaoo/tadk/tool"
	"google.golang.org/genai"
)

const defaultInstruction = "You are an interactive terminal coding assistant. Be concise, use tools when useful, explain tradeoffs clearly, and avoid destructive actions unless the user explicitly asks for them."

type ModelChoice struct {
	Provider string
	Name     string
}

func (m ModelChoice) Valid() bool {
	return strings.TrimSpace(m.Provider) != "" && strings.TrimSpace(m.Name) != ""
}

func (m ModelChoice) Label() string {
	return fmt.Sprintf("%s / %s", m.Provider, m.Name)
}

type Factory struct {
	ConfigPath string
	Config     config.Config
	Tools      []adktool.Tool
}

type RunnerBundle struct {
	Runner *runner.Runner
	Choice ModelChoice
}

func FindConfig(startDir string) (string, error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, config.DefaultPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find %s from %s upward", config.DefaultPath, startDir)
		}
		dir = parent
	}
}

func NewFactory(configPath string, tools []adktool.Tool) (*Factory, error) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return nil, err
	}
	return &Factory{
		ConfigPath: configPath,
		Config:     cfg,
		Tools:      slices.Clone(tools),
	}, nil
}

func (f *Factory) DefaultChoice() ModelChoice {
	return ModelChoice{
		Provider: f.Config.Model.Provider,
		Name:     f.Config.Model.Name,
	}
}

func (f *Factory) AvailableProviders() []string {
	providers := make([]string, 0, len(f.Config.Providers))
	for name := range f.Config.Providers {
		providers = append(providers, name)
	}
	slices.Sort(providers)
	return providers
}

func (f *Factory) NormalizeChoice(choice ModelChoice) ModelChoice {
	choice.Provider = strings.TrimSpace(strings.ToLower(choice.Provider))
	choice.Name = strings.TrimSpace(choice.Name)
	return choice
}

func (f *Factory) HasProvider(provider string) bool {
	_, ok := f.Config.Provider(provider)
	return ok
}

func (f *Factory) Build(ctx context.Context, choice ModelChoice, sessionService session.Service, appName string) (*RunnerBundle, error) {
	choice = f.NormalizeChoice(choice)
	if !choice.Valid() {
		return nil, fmt.Errorf("provider and model are required")
	}

	providerCfg, ok := f.Config.Provider(choice.Provider)
	if !ok {
		return nil, fmt.Errorf("provider %q not found in %s", choice.Provider, f.ConfigPath)
	}

	adapter, err := buildAdapter(ctx, choice.Provider, providerCfg)
	if err != nil {
		return nil, err
	}

	root, err := llmagent.New(llmagent.Config{
		Name:        "tui",
		Description: "Interactive terminal assistant",
		Model:       adapter,
		ModelID:     choice.Name,
		Config: model.DefaultGenerateConfig().
			WithTemperature(0.2).
			WithToolChoice(model.ToolChoice{Mode: model.ToolChoiceAuto}),
		Instruction: defaultInstruction,
		Tools:       slices.Clone(f.Tools),
	})
	if err != nil {
		return nil, err
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          root,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, err
	}

	return &RunnerBundle{
		Runner: r,
		Choice: choice,
	}, nil
}

func buildAdapter(ctx context.Context, provider string, cfg config.ProviderConfig) (model.ModelAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return anthropicadapter.New(anthropicadapter.Config{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
		}), nil
	case "google", "gemini":
		clientCfg := &genai.ClientConfig{
			APIKey:  cfg.APIKey,
			Backend: genai.BackendGeminiAPI,
		}
		if cfg.BaseURL != "" {
			clientCfg.HTTPOptions.BaseURL = cfg.BaseURL
		}
		return geminiadapter.New(ctx, geminiadapter.Config{ClientConfig: clientCfg})
	default:
		return openaiadapter.New(openaiadapter.Config{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
		}), nil
	}
}
