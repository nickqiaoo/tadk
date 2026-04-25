package processors

import (
	llmconfig "github.com/nickqiaoo/tadk/internal/llminternal/config"
	"github.com/nickqiaoo/tadk/model"
)

// ResolveBasic produces the invariant generation parameters for a request:
// the model identifier and a fresh GenerateConfig derived from cfg. The
// returned GenerateConfig is owned by the caller and embedded in a cached
// prefix; downstream consumers (per-step requests, adapters) must treat it as
// read-only — see model.Request.Config.
func ResolveBasic(cfg *llmconfig.Config) (modelID string, gen *model.GenerateConfig) {
	gen = model.CloneGenerateConfig(cfg.Config)
	if gen == nil {
		gen = &model.GenerateConfig{}
	}
	if cfg.OutputSchema != nil {
		gen.ResponseSchema = cfg.OutputSchema
		gen.ResponseMIMEType = "application/json"
	}
	return cfg.ModelID, gen
}
