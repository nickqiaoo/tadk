package processors

import (
	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/tool"
)

func AugmentRequest(ctx agent.InvocationContext, req *model.Request, tools []tool.Tool) error {
	toolCtx := toolinternal.NewToolContext(ctx, "")
	for _, t := range tools {
		augmenter, ok := t.(toolinternal.RequestAugmenter)
		if !ok {
			continue
		}
		if err := augmenter.AugmentRequest(toolCtx, req); err != nil {
			return err
		}
	}
	return nil
}
