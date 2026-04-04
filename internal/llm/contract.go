package llm

import "context"

type InternalRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolDefinition
	Assets   map[AssetID]ImageAsset
	Stream   bool
}

type StreamEventType string

const (
	EventTextDelta StreamEventType = "text_delta"
	EventToolUse   StreamEventType = "tool_use"
	EventThinking  StreamEventType = "thinking"
	EventDone      StreamEventType = "done"
	EventError     StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType
	Text    string
	ToolUse *ToolUsePart
	Err     *ProviderError
}

type ProviderClient interface {
	Chat(ctx context.Context, req InternalRequest) (<-chan StreamEvent, error)
}
