package tools

import (
	"context"
	"encoding/json"
)

// Registry provides discovery for available tools.
type Registry interface {
	Get(name string) (Tool, bool)
	List() []ToolMeta
}

// Tool defines a callable unit in tool layer.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage, tctx ToolUseContext) (<-chan ToolEvent, error)
}

// ToolMeta describes a tool entry.
type ToolMeta struct {
	Name             string
	SideEffectLevel  string
	IdempotencyLevel string
}

// ToolUseContext carries runtime metadata for a tool call.
type ToolUseContext struct {
	SessionID string
	TaskID    string
}

// ToolEventType is the event kind used by the unified tool stream.
type ToolEventType string

const (
	ToolEventStart  ToolEventType = "start"
	ToolEventChunk  ToolEventType = "chunk"
	ToolEventResult ToolEventType = "result"
	ToolEventError  ToolEventType = "error"
)

// ToolEvent is the standard output unit from tools.
type ToolEvent struct {
	Type ToolEventType
	Data json.RawMessage
}
