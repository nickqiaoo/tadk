// Package main demonstrates wiring an LLMAgent into the standard launcher:
//
//	Launcher -> Runner -> LLMAgent turn loop -> Flow.Step -> ModelAdapter.Stream
//	-> AgentEvent stream + result() -> []*message.Message
//
// Run:
//
//	cp ./examples/llmagent/adk.toml.example ./adk.toml
//	go run ./examples/llmagent
package main

import (
	"context"
	"log"
	"os"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/cmd/launcher"
	"github.com/nickqiaoo/tadk/cmd/launcher/full"
	"github.com/nickqiaoo/tadk/config"
	"github.com/nickqiaoo/tadk/model"
	geminiadapter "github.com/nickqiaoo/tadk/model/gemini"
	"github.com/nickqiaoo/tadk/session"
	adktool "github.com/nickqiaoo/tadk/tool"
	"github.com/nickqiaoo/tadk/tool/functiontool"
	"google.golang.org/genai"
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"The city to look up."`
}

type weatherResult struct {
	City      string `json:"city"`
	Condition string `json:"condition"`
	TempC     int    `json:"temp_c"`
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	providerCfg, ok := cfg.Provider(cfg.Model.Provider)
	if !ok {
		log.Fatalf("provider config for %q not found", cfg.Model.Provider)
	}
	if cfg.Model.Provider != "google" {
		log.Fatalf("example only supports provider %q, got %q", "google", cfg.Model.Provider)
	}

	weatherTool, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Returns a small deterministic weather report for a city.",
	}, func(ctx adktool.Context, args weatherArgs) (weatherResult, *adktool.Control, error) {
		city := args.City
		if city == "" {
			city = "unknown"
		}
		return weatherResult{
			City:      city,
			Condition: "sunny",
			TempC:     24,
		}, nil, nil
	})
	if err != nil {
		log.Fatalf("create weather tool: %v", err)
	}

	geminiModel, err := geminiadapter.New(ctx, geminiadapter.Config{
		ClientConfig: &genai.ClientConfig{
			APIKey:  providerCfg.APIKey,
			Backend: genai.BackendGeminiAPI,
		},
	})
	if err != nil {
		log.Fatalf("create gemini model: %v", err)
	}

	root, err := llmagent.New(llmagent.Config{
		Name:        "weather_agent",
		Description: "Answers weather questions using a local weather tool.",
		Model:       geminiModel,
		ModelID:     cfg.Model.Name,
		Config: model.DefaultGenerateConfig().
			WithTemperature(0.2).
			WithToolChoice(model.ToolChoice{Mode: model.ToolChoiceAuto}),
		Instruction: "You are a concise assistant. Use tools when they are useful, then answer in one short paragraph.",
		Tools:       []adktool.Tool{weatherTool},
	})
	if err != nil {
		log.Fatalf("create llm agent: %v", err)
	}

	sessionService := session.InMemoryService()
	cfgLauncher := full.NewLauncher()
	if err := cfgLauncher.Execute(ctx, &launcher.Config{
		SessionService: sessionService,
		AgentLoader:    agent.NewSingleLoader(root),
		Preset:         launcher.PresetLocal,
	}, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
