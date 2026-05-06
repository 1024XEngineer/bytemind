package provider

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
)

func TestModelRegistryContextWindow(t *testing.T) {
	registry := NewModelRegistry(config.ProviderRuntimeConfig{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-5.4-mini",
		Providers: map[string]config.ProviderConfig{
			"openai": {
				Model:  "gpt-5.4-mini",
				Family: "openai",
			},
		},
	}, []ModelInfo{{
		ProviderID: "openai",
		ModelID:    "gpt-5.4-mini",
		Metadata: map[string]string{
			"context_window": "128000",
		},
	}})

	if got := registry.ContextWindow("openai", "gpt-5.4-mini"); got != 128000 {
		t.Fatalf("expected context window 128000, got %d", got)
	}
}
