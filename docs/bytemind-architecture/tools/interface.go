package tools

import "context"

// Registry provides discovery for available tools.
type Registry interface {
	Get(name string) (Tool, bool)
	List() []ToolMeta
}

// Tool defines a callable unit in tool layer.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args []byte, tctx UseContext) (<-chan Event, error)
}

// ToolMeta describes a tool entry.
type ToolMeta struct {
	Name string
}

// UseContext carries runtime metadata for a tool call.
type UseContext struct {
	SessionID string
	TaskID    string
}

// Event is the standard output unit from tools.
type Event struct {
	Type string
	Data []byte
}
