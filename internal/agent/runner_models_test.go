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
