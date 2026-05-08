package tui

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

func TestRenderModelsModalSwitchModeIncludesFlagsAndMetadata(t *testing.T) {
	m := model{
		width:           120,
		commandCursor:   0,
		modelPickerMode: modelPickerModeSwitch,
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "gpt-5.4"},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", Model: "gpt-5.4"},
				},
			},
		},
		discoveredModels: []provider.ModelInfo{{
			ProviderID: "openai",
			ModelID:    "gpt-5.4",
			Metadata: map[string]string{
				"family":         "gpt",
				"context_window": "128000",
				"usage_source":   "metadata",
			},
		}},
		modelWarnings: []provider.Warning{{ProviderID: "deepseek", Reason: "provider_list_models_failed"}},
	}

	view := m.renderModelsModal()
	for _, want := range []string{"Models", "Current: openai/gpt-5.4", "openai/gpt-5.4  (active, default)", "family=gpt", "context=128000", "source=metadata", "Warnings:", "deepseek: provider_list_models_failed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected models modal to contain %q, got %q", want, view)
		}
	}
}

func TestRenderModelsModalDeleteModeAndEmptyStates(t *testing.T) {
	m := model{
		width:           120,
		modelPickerMode: modelPickerModeDelete,
	}
	view := m.renderModelsModal()
	if !strings.Contains(view, "Delete Model") || !strings.Contains(view, "No configured models available to delete.") {
		t.Fatalf("expected delete empty state, got %q", view)
	}

	m.modelPickerMode = modelPickerModeSwitch
	view = m.renderModelsModal()
	if !strings.Contains(view, "No switchable models available.") {
		t.Fatalf("expected switch empty state, got %q", view)
	}
}
