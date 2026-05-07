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

func TestModelRegistryMergesNormalizesAndCopiesModels(t *testing.T) {
	registry := NewModelRegistry(config.ProviderRuntimeConfig{
		Providers: map[string]config.ProviderConfig{
			" OpenAI ": {
				Model:  " gpt-5.4-mini ",
				Family: " openai ",
			},
			"empty": {
				Model: " ",
			},
		},
	}, []ModelInfo{
		{
			ProviderID:   " OpenAI ",
			ModelID:      " gpt-5.4 ",
			DisplayAlias: " GPT ",
			Metadata:     map[string]string{"source": "", "supports_tools": ""},
		},
		{
			ProviderID: "openai",
			ModelID:    "gpt-5.4",
		},
		{
			ProviderID: "",
			ModelID:    "ignored",
		},
	})

	models := registry.Models()
	if len(models) != 2 {
		t.Fatalf("expected discovered and config fallback models, got %#v", models)
	}
	if models[0].ProviderID != "openai" || models[0].ModelID != "gpt-5.4" {
		t.Fatalf("expected normalized discovered model first, got %#v", models[0])
	}
	if models[0].DisplayAlias != "GPT" {
		t.Fatalf("expected display alias to be trimmed, got %q", models[0].DisplayAlias)
	}
	if models[0].Metadata["source"] != "provider" || models[0].Metadata["supports_tools"] != "true" {
		t.Fatalf("expected discovered metadata defaults, got %#v", models[0].Metadata)
	}
	if models[1].ModelID != "gpt-5.4-mini" {
		t.Fatalf("expected config fallback model, got %#v", models[1])
	}
	if models[1].Metadata["source"] != "config" || models[1].Metadata["family"] != "openai" {
		t.Fatalf("expected config metadata, got %#v", models[1].Metadata)
	}

	models[0].ModelID = "mutated"
	copiedAgain := registry.Models()
	if copiedAgain[0].ModelID != "gpt-5.4" {
		t.Fatalf("expected Models to return a copy, got %#v", copiedAgain)
	}

	if _, ok := registry.Lookup(" OPENAI ", " gpt-5.4 "); !ok {
		t.Fatal("expected lookup to normalize provider and model ids")
	}
	if _, ok := registry.Lookup("openai", "missing"); ok {
		t.Fatal("expected missing lookup to return false")
	}
	if got := registry.ContextWindow("openai", "missing"); got != 0 {
		t.Fatalf("expected missing context window to be zero, got %d", got)
	}
}

func TestModelRegistryKeyRejectsEmptyParts(t *testing.T) {
	if got := modelRegistryKey("", "model"); got != "" {
		t.Fatalf("expected empty provider key to be rejected, got %q", got)
	}
	if got := modelRegistryKey("provider", " "); got != "" {
		t.Fatalf("expected empty model key to be rejected, got %q", got)
	}
	if got := modelRegistryKey(" Provider ", " model "); got != "provider/model" {
		t.Fatalf("expected normalized key, got %q", got)
	}
}
