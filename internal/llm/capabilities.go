package llm

import "strings"

type ModelCapabilities struct {
	SupportsVision   bool
	SupportsToolUse  bool
	SupportsThinking bool
}

type CapabilityRegistry struct {
	static   map[string]ModelCapabilities
	override map[string]ModelCapabilities
	runtime  map[string]ModelCapabilities
}

func NewCapabilityRegistry(static map[string]ModelCapabilities) *CapabilityRegistry {
	registry := &CapabilityRegistry{
		static:   make(map[string]ModelCapabilities, len(static)),
		override: make(map[string]ModelCapabilities, 8),
		runtime:  make(map[string]ModelCapabilities, 8),
	}
	for model, caps := range static {
		registry.static[normalizeModelKey(model)] = caps
	}
	return registry
}

func (r *CapabilityRegistry) Resolve(model string) ModelCapabilities {
	key := normalizeModelKey(model)
	if key == "" {
		return defaultCapabilities()
	}
	if caps, ok := r.override[key]; ok {
		return caps
	}
	if caps, ok := r.runtime[key]; ok {
		return caps
	}
	if caps, ok := r.static[key]; ok {
		return caps
	}
	return inferCapabilitiesFromModel(key)
}

func (r *CapabilityRegistry) SetOverride(model string, caps ModelCapabilities) {
	key := normalizeModelKey(model)
	if key == "" {
		return
	}
	r.override[key] = caps
}

func (r *CapabilityRegistry) Learn(model string, caps ModelCapabilities) {
	key := normalizeModelKey(model)
	if key == "" {
		return
	}
	r.runtime[key] = caps
}

var DefaultModelCapabilities = NewCapabilityRegistry(map[string]ModelCapabilities{
	"gpt-4o":          {SupportsVision: true, SupportsToolUse: true, SupportsThinking: false},
	"gpt-4.1":         {SupportsVision: true, SupportsToolUse: true, SupportsThinking: false},
	"gpt-5.4":         {SupportsVision: true, SupportsToolUse: true, SupportsThinking: true},
	"gpt-5.4-mini":    {SupportsVision: true, SupportsToolUse: true, SupportsThinking: true},
	"claude-sonnet-4": {SupportsVision: true, SupportsToolUse: true, SupportsThinking: true},
})

func ApplyCapabilities(messages []Message, caps ModelCapabilities) []Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		message.Normalize()
		parts := make([]Part, 0, len(message.Parts))
		for _, part := range message.Parts {
			switch part.Type {
			case PartImageRef:
				if caps.SupportsVision {
					parts = append(parts, part)
				}
			case PartToolUse, PartToolResult:
				if caps.SupportsToolUse {
					parts = append(parts, part)
				}
			case PartThinking:
				if caps.SupportsThinking {
					parts = append(parts, part)
				} else if part.Thinking != nil {
					parts = append(parts, Part{Type: PartText, Text: &TextPart{Value: part.Thinking.Value}})
				}
			default:
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			parts = append(parts, Part{Type: PartText, Text: &TextPart{Value: "(content omitted due to model capability limits)"}})
		}
		message.Parts = parts
		message.Content = ""
		message.ToolCallID = ""
		message.ToolCalls = nil
		message.Normalize()
		result = append(result, message)
	}
	return result
}

func normalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func defaultCapabilities() ModelCapabilities {
	return ModelCapabilities{SupportsVision: false, SupportsToolUse: true, SupportsThinking: true}
}

func inferCapabilitiesFromModel(model string) ModelCapabilities {
	caps := defaultCapabilities()
	if strings.Contains(model, "4o") || strings.Contains(model, "vision") || strings.Contains(model, "gpt-5") || strings.Contains(model, "claude") {
		caps.SupportsVision = true
	}
	if strings.Contains(model, "no-tool") {
		caps.SupportsToolUse = false
	}
	if strings.Contains(model, "mini") && strings.Contains(model, "gpt-4") {
		caps.SupportsThinking = false
	}
	return caps
}
