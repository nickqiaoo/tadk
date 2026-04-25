package temporal

import (
	"github.com/nickqiaoo/tadk/session"
	"github.com/nickqiaoo/tadk/tool"
)

// ToolState is the queryable runtime state for a Temporal-backed durable tool.
type ToolState struct {
	Status            Status                               `json:"status"`
	Result            map[string]any                       `json:"result,omitempty"`
	Control           *tool.Control                        `json:"control,omitempty"`
	PendingInterrupts map[string]*session.PendingInterrupt `json:"pendingInterrupts,omitempty"`
	Error             string                               `json:"error,omitempty"`
}

// ToolResult is the terminal workflow result for a Temporal-backed durable tool.
type ToolResult struct {
	Result    map[string]any `json:"result,omitempty"`
	Control   *tool.Control  `json:"control,omitempty"`
	Error     string         `json:"error,omitempty"`
	Cancelled bool           `json:"cancelled,omitempty"`
}
