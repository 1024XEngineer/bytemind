package llm

import "testing"

func TestCapabilityRegistryResolveWithOverrideAndLearn(t *testing.T) {
	registry := NewCapabilityRegistry(map[string]ModelCapabilities{
		"model-a": {SupportsVision: false, SupportsToolUse: true, SupportsThinking: true},
	})

	caps := registry.Resolve("model-a")
	if caps.SupportsVision {
		t.Fatalf("expected static caps, got %#v", caps)
	}

	registry.Learn("model-a", ModelCapabilities{SupportsVision: true, SupportsToolUse: true, SupportsThinking: true})
	caps = registry.Resolve("model-a")
	if !caps.SupportsVision {
		t.Fatalf("expected learned caps, got %#v", caps)
	}

	registry.SetOverride("model-a", ModelCapabilities{SupportsVision: false, SupportsToolUse: false, SupportsThinking: false})
	caps = registry.Resolve("model-a")
	if caps.SupportsToolUse || caps.SupportsThinking {
		t.Fatalf("expected override to win, got %#v", caps)
	}
}

func TestApplyCapabilitiesDegradesThinkingAndImage(t *testing.T) {
	messages := []Message{{
		Role: RoleAssistant,
		Parts: []Part{
			{Type: PartThinking, Thinking: &ThinkingPart{Value: "private thought"}},
			{Type: PartToolUse, ToolUse: &ToolUsePart{ID: "call-1", Name: "x", Arguments: "{}"}},
		},
	}}

	out := ApplyCapabilities(messages, ModelCapabilities{
		SupportsVision:   false,
		SupportsToolUse:  false,
		SupportsThinking: false,
	})
	if len(out) != 1 {
		t.Fatalf("unexpected message count: %d", len(out))
	}
	if len(out[0].Parts) != 1 || out[0].Parts[0].Type != PartText {
		t.Fatalf("expected thinking downgraded to text only, got %#v", out[0].Parts)
	}
	if out[0].Text() != "private thought" {
		t.Fatalf("unexpected text: %q", out[0].Text())
	}
}

func TestCapabilityRegistryResolveUsesInferenceFallback(t *testing.T) {
	registry := NewCapabilityRegistry(nil)
	caps := registry.Resolve("gpt-5.4-no-tool")
	if !caps.SupportsVision || caps.SupportsToolUse {
		t.Fatalf("unexpected inferred caps: %#v", caps)
	}

	empty := registry.Resolve("   ")
	if empty.SupportsVision || !empty.SupportsToolUse || !empty.SupportsThinking {
		t.Fatalf("unexpected default caps: %#v", empty)
	}
}

func TestDefaultModelCapabilitiesRecognizeQwenVisionModels(t *testing.T) {
	for _, model := range []string{
		"qwen3.6-flash",
		"qwen3.6-plus",
		"qwen3.6-pro",
		"qwen/qwen3.6-plus",
		"dashscope/qwen3.6-flash",
		"qwen3-vl-flash",
		"qwen2.5-vl-72b-instruct",
	} {
		if caps := DefaultModelCapabilities.Resolve(model); !caps.SupportsVision {
			t.Fatalf("expected %s to support vision, got %#v", model, caps)
		}
	}
}

func TestDefaultModelCapabilitiesDoesNotMarkGenericQwenTextModelAsVision(t *testing.T) {
	if caps := DefaultModelCapabilities.Resolve("qwen3-coder-plus"); caps.SupportsVision {
		t.Fatalf("expected generic qwen text model not to support vision, got %#v", caps)
	}
	if caps := DefaultModelCapabilities.Resolve("qwen/qwen3-coder-plus"); caps.SupportsVision {
		t.Fatalf("expected provider-prefixed generic qwen text model not to support vision, got %#v", caps)
	}
}

func TestApplyCapabilitiesAddsFallbackTextWhenAllPartsDropped(t *testing.T) {
	out := ApplyCapabilities([]Message{{
		Role: RoleUser,
		Parts: []Part{{
			Type:  PartImageRef,
			Image: &ImagePartRef{AssetID: "asset-1"},
		}},
	}}, ModelCapabilities{
		SupportsVision:   false,
		SupportsToolUse:  true,
		SupportsThinking: true,
	})

	if len(out) != 1 || len(out[0].Parts) != 1 || out[0].Parts[0].Type != PartText {
		t.Fatalf("expected fallback text part, got %#v", out)
	}
	if out[0].Text() == "" {
		t.Fatalf("expected non-empty fallback text, got %#v", out[0])
	}
}
