package provider

import (
	"context"
	"testing"

	"bytemind/internal/config"
)

func TestListModelsUsesRegistryInstanceIDForAllResults(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: "custom-openai", models: []ModelInfo{{ProviderID: "openai-compatible", ModelID: "gpt-5.4"}, {ProviderID: "other", ModelID: "gpt-4.1"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
	if len(models) != 2 {
		t.Fatalf("unexpected models %#v", models)
	}
	for _, model := range models {
		if model.ProviderID != "custom-openai" {
			t.Fatalf("expected registry instance id, got %#v", model)
		}
	}
}
