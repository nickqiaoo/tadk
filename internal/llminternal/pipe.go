package llminternal

import (
	"iter"

	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/tool"
)

// EventPipe 是一个有 finalize 能力的显式事件流。
type EventPipe struct {
	Events   iter.Seq2[event.Event, error]
	Finalize func() (PipeResult, error)
}

// PipeResult 是 pipe 执行完毕后暴露的显式状态。
type PipeResult struct {
	LastMessage *message.Message
	Control     *tool.Control
	Err         error
}
