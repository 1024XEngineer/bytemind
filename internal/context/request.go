package context

import "github.com/1024XEngineer/bytemind/internal/llm"

// ChatRequestInput describes one model request assembly unit from prepared
// turn messages and candidate tools.
type ChatRequestInput struct {
	Model       string
	Messages    []llm.Message
	Tools       []llm.ToolDefinition
	Assets      map[llm.AssetID]llm.ImageAsset
	Temperature float64
}

// BuildChatRequest applies model capability gates and returns a ready request.
func BuildChatRequest(in ChatRequestInput) llm.ChatRequest {
	caps := llm.DefaultModelCapabilities.Resolve(in.Model)
	requestTools := in.Tools
	if !caps.SupportsToolUse {
		requestTools = nil
	}
	return llm.ChatRequest{
		Model:       in.Model,
		Messages:    llm.ApplyCapabilities(in.Messages, caps),
		Tools:       requestTools,
		Assets:      in.Assets,
		Temperature: in.Temperature,
	}
}
