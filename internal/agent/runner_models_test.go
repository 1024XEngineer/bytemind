package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

func TestRunnerListModelsUsesConfiguredRuntime(t *testing.T) {
	runner := NewRunner(Options{
		Config: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "primary",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"primary": {
						Type:  "openai-compatible",
						Model: "gpt-5.4",
					},
				},
			},
		},
		Client: &fakeClient{},
	})

	models, warnings, err := runner.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected models to list, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
	if len(models) != 1 {
		t.Fatalf("expected one model, got %#v", models)
	}
	if models[0].ProviderID != "primary" || models[0].ModelID != "gpt-5.4" {
		t.Fatalf("unexpected model %#v", models[0])
	}
}

func TestRunnerListModelsFallsBackToLegacyProviderConfig(t *testing.T) {
	runner := NewRunner(Options{
		Config: config.Config{
			Provider: config.ProviderConfig{
				Type:  "openai-compatible",
				Model: "legacy-model",
			},
		},
		Client: &fakeClient{},
	})

	models, warnings, err := runner.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected legacy models to list, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
	if len(models) != 1 || models[0].ProviderID != provider.ProviderOpenAI || models[0].ModelID != "legacy-model" {
		t.Fatalf("unexpected legacy models %#v", models)
	}
}

func TestRunnerListModelsUsesCacheAndCopiesResults(t *testing.T) {
	runner := NewRunner(Options{
		Config: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "primary",
				DefaultModel:    "cached-model",
				Providers: map[string]config.ProviderConfig{
					"primary": {Type: "openai-compatible", Model: "fresh-model"},
				},
			},
		},
		Client: &fakeClient{},
	})
	runner.storeModelsCache(
		[]provider.ModelInfo{{ProviderID: "primary", ModelID: "cached-model"}},
		[]provider.Warning{{ProviderID: "primary", Reason: "cached-warning"}},
	)

	models, warnings, err := runner.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected cached models, got %v", err)
	}
	if len(models) != 1 || models[0].ModelID != "cached-model" {
		t.Fatalf("expected cached model, got %#v", models)
	}
	if len(warnings) != 1 || warnings[0].Reason != "cached-warning" {
		t.Fatalf("expected cached warning, got %#v", warnings)
	}

	models[0].ModelID = "mutated"
	warnings[0].Reason = "mutated"
	models, warnings, ok := runner.listModelsFromCache()
	if !ok {
		t.Fatal("expected cache hit")
	}
	if models[0].ModelID != "cached-model" || warnings[0].Reason != "cached-warning" {
		t.Fatalf("expected cache copies to protect stored state, got models=%#v warnings=%#v", models, warnings)
	}

	runner.modelsCacheAt = time.Now().Add(-31 * time.Second)
	if _, _, ok := runner.listModelsFromCache(); ok {
		t.Fatal("expected expired cache miss")
	}
}

func TestRunnerListModelsRejectsInvalidState(t *testing.T) {
	if _, _, err := (*Runner)(nil).ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "client is unavailable") {
		t.Fatalf("expected nil runner error, got %v", err)
	}

	runner := NewRunner(Options{
		Config: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "missing",
				Providers: map[string]config.ProviderConfig{
					"primary": {Type: "openai-compatible", Model: "model"},
				},
			},
		},
		Client: &fakeClient{},
	})
	if _, _, err := runner.ListModels(context.Background()); err == nil {
		t.Fatal("expected invalid provider runtime to fail")
	}
}

func TestRunnerUpdateProviderSyncsRuntimeAndClearsModelCache(t *testing.T) {
	runner := NewRunner(Options{
		Config: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "old",
				DefaultModel:    "old-model",
				Providers: map[string]config.ProviderConfig{
					"old": {Type: "openai-compatible", Model: "old-model"},
				},
			},
		},
		Client: &fakeClient{},
	})
	runner.storeModelsCache(
		[]provider.ModelInfo{{ProviderID: "old", ModelID: "old-model"}},
		[]provider.Warning{{ProviderID: "old", Reason: "old-warning"}},
	)

	nextClient := &fakeClient{}
	runner.UpdateProvider(config.ProviderConfig{
		Type:  "anthropic",
		Model: "claude-sonnet-4",
	}, nextClient)

	if _, _, ok := runner.listModelsFromCache(); ok {
		t.Fatal("expected provider update to clear model cache")
	}
	if runner.config.ProviderRuntime.DefaultProvider != "anthropic" {
		t.Fatalf("expected runtime default provider to sync, got %q", runner.config.ProviderRuntime.DefaultProvider)
	}
	if runner.config.ProviderRuntime.DefaultModel != "claude-sonnet-4" {
		t.Fatalf("expected runtime default model to sync, got %q", runner.config.ProviderRuntime.DefaultModel)
	}
	if _, ok := runner.config.ProviderRuntime.Providers["old"]; !ok {
		t.Fatalf("expected existing providers to be preserved, got %#v", runner.config.ProviderRuntime.Providers)
	}
	if runner.config.ProviderRuntime.Providers["anthropic"].Model != "claude-sonnet-4" {
		t.Fatalf("expected anthropic provider entry to be updated, got %#v", runner.config.ProviderRuntime.Providers["anthropic"])
	}
	if got := runner.GetClient(); got != nextClient {
		t.Fatalf("expected GetClient to unwrap updated route-aware client")
	}
}

func TestRunnerUpdateProviderRuntimePreservesSelectedProviderIDAndClearsCache(t *testing.T) {
	runner := NewRunner(Options{
		Config: config.Config{
			Provider: config.ProviderConfig{
				Type:  "openai-compatible",
				Model: "gpt-5.4-mini",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4-mini",
				Providers: map[string]config.ProviderConfig{
					"openai":   {Type: "openai-compatible", Model: "gpt-5.4-mini"},
					"deepseek": {Type: "openai-compatible", Model: "deepseek-chat"},
				},
			},
		},
		Client: &fakeClient{},
	})
	runner.storeModelsCache(
		[]provider.ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4-mini"}},
		[]provider.Warning{{ProviderID: "openai", Reason: "cached"}},
	)

	nextClient := &fakeClient{}
	nextRuntime := config.ProviderRuntimeConfig{
		DefaultProvider: "deepseek",
		DefaultModel:    "deepseek-reasoner",
		Providers: map[string]config.ProviderConfig{
			"openai":   {Type: "openai-compatible", Model: "gpt-5.4-mini"},
			"deepseek": {Type: "openai-compatible", Model: "deepseek-reasoner"},
		},
	}
	nextProvider := config.ProviderConfig{
		Type:  "openai-compatible",
		Model: "deepseek-reasoner",
	}
	runner.UpdateProviderRuntime(nextRuntime, nextProvider, nextClient)

	if _, _, ok := runner.listModelsFromCache(); ok {
		t.Fatal("expected runtime update to clear model cache")
	}
	if runner.config.ProviderRuntime.DefaultProvider != "deepseek" || runner.config.ProviderRuntime.DefaultModel != "deepseek-reasoner" {
		t.Fatalf("expected runtime defaults to preserve selected provider id, got %#v", runner.config.ProviderRuntime)
	}
	if runner.config.Provider.Model != "deepseek-reasoner" {
		t.Fatalf("expected legacy provider config to sync to selected model, got %#v", runner.config.Provider)
	}
	if got := runner.GetClient(); got != nextClient {
		t.Fatalf("expected GetClient to unwrap updated runtime client")
	}
}
