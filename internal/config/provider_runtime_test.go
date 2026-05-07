package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProviderRuntimeConfigFile(t *testing.T, workspace string, cfg map[string]any) {
	t.Helper()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	projectConfigDir := filepath.Join(workspace, ".bytemind")
	if err := os.MkdirAll(projectConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigLoadPreservesExplicitProviderRuntime(t *testing.T) {
	workspace := t.TempDir()
	writeProviderRuntimeConfigFile(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"provider_runtime": map[string]any{
			"default_provider": "openai",
			"default_model":    "gpt-5.4-mini",
			"allow_fallback":   true,
			"providers": map[string]any{
				"openai": map[string]any{
					"type":     "openai-compatible",
					"base_url": "https://api.openai.com/v1",
					"model":    "gpt-5.4-mini",
					"api_key":  "test-key",
				},
			},
		},
	})
	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderRuntime.DefaultProvider != "openai" || cfg.ProviderRuntime.DefaultModel != "gpt-5.4-mini" || !cfg.ProviderRuntime.AllowFallback {
		t.Fatalf("unexpected provider runtime %#v", cfg.ProviderRuntime)
	}
	if len(cfg.ProviderRuntime.Providers) != 1 {
		t.Fatalf("unexpected provider runtime providers %#v", cfg.ProviderRuntime.Providers)
	}
}

func TestLegacyProviderRuntimeConfigNormalizesProviderIDs(t *testing.T) {
	tests := []struct {
		name      string
		typeValue string
		want      string
	}{
		{name: "openai compatible", typeValue: "openai-compatible", want: "openai"},
		{name: "openai alias", typeValue: "openai", want: "openai"},
		{name: "empty defaults openai", typeValue: "", want: "openai"},
		{name: "openai uppercase", typeValue: "OPENAI", want: "openai"},
		{name: "openai compatible padded", typeValue: " OpenAI-Compatible ", want: "openai"},
		{name: "anthropic uppercase", typeValue: "ANTHROPIC", want: "anthropic"},
		{name: "anthropic", typeValue: "anthropic", want: "anthropic"},
		{name: "gemini uppercase", typeValue: "GEMINI", want: "gemini"},
		{name: "gemini", typeValue: "gemini", want: "gemini"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ProviderConfig{Type: tt.typeValue, Model: "test-model"}
			runtime := LegacyProviderRuntimeConfig(cfg)
			if runtime.DefaultProvider != tt.want {
				t.Fatalf("unexpected default provider %q", runtime.DefaultProvider)
			}
			if runtime.DefaultModel != "test-model" {
				t.Fatalf("unexpected default model %q", runtime.DefaultModel)
			}
			if len(runtime.Providers) != 1 || runtime.Providers[tt.want].Type != tt.want {
				t.Fatalf("unexpected providers %#v", runtime.Providers)
			}
		})
	}
}

func TestSyncProviderRuntimeWithProviderUpdatesDefaultAndPreservesOthers(t *testing.T) {
	runtime := SyncProviderRuntimeWithProvider(ProviderRuntimeConfig{
		DefaultProvider: "old",
		DefaultModel:    "old-model",
		AllowFallback:   true,
		Providers: map[string]ProviderConfig{
			"old": {Type: "openai-compatible", Model: "old-model"},
		},
	}, ProviderConfig{
		Type:      "anthropic",
		BaseURL:   "https://api.anthropic.com",
		Model:     "claude-sonnet-4",
		APIKeyEnv: "ANTHROPIC_API_KEY",
	})

	if runtime.DefaultProvider != "anthropic" {
		t.Fatalf("expected default provider to sync to anthropic, got %q", runtime.DefaultProvider)
	}
	if runtime.DefaultModel != "claude-sonnet-4" {
		t.Fatalf("expected default model to sync, got %q", runtime.DefaultModel)
	}
	if !runtime.AllowFallback {
		t.Fatal("expected existing allow_fallback setting to be preserved")
	}
	if _, ok := runtime.Providers["old"]; !ok {
		t.Fatalf("expected existing provider entry to be preserved, got %#v", runtime.Providers)
	}
	anthropic := runtime.Providers["anthropic"]
	if anthropic.Model != "claude-sonnet-4" || anthropic.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Fatalf("expected synced anthropic provider entry, got %#v", anthropic)
	}
}

func TestSyncProviderRuntimeWithProviderBuildsMissingProvidersMap(t *testing.T) {
	runtime := SyncProviderRuntimeWithProvider(ProviderRuntimeConfig{}, ProviderConfig{
		Type:  "openai-compatible",
		Model: "gpt-5.4",
	})

	if runtime.DefaultProvider != "openai" || runtime.DefaultModel != "gpt-5.4" {
		t.Fatalf("unexpected runtime defaults %#v", runtime)
	}
	if providerCfg, ok := runtime.Providers["openai"]; !ok || providerCfg.Type != "openai" || providerCfg.Model != "gpt-5.4" {
		t.Fatalf("unexpected provider entry %#v", runtime.Providers)
	}
}

func TestSelectProviderRuntimeModelUpdatesSelectedProviderOnly(t *testing.T) {
	runtime, providerCfg, err := SelectProviderRuntimeModel(ProviderRuntimeConfig{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-5.4-mini",
		Providers: map[string]ProviderConfig{
			"openai":   {Type: "openai-compatible", Model: "gpt-5.4-mini", APIKeyEnv: "OPENAI_API_KEY"},
			"deepseek": {Type: "openai-compatible", Model: "deepseek-chat", APIKeyEnv: "DEEPSEEK_API_KEY"},
		},
	}, "deepseek", "deepseek-reasoner")
	if err != nil {
		t.Fatalf("expected runtime selection to succeed, got %v", err)
	}
	if runtime.DefaultProvider != "deepseek" || runtime.DefaultModel != "deepseek-reasoner" {
		t.Fatalf("unexpected runtime defaults %#v", runtime)
	}
	if runtime.Providers["openai"].Model != "gpt-5.4-mini" {
		t.Fatalf("expected non-selected provider to remain unchanged, got %#v", runtime.Providers["openai"])
	}
	if runtime.Providers["deepseek"].Model != "deepseek-reasoner" {
		t.Fatalf("expected selected provider model update, got %#v", runtime.Providers["deepseek"])
	}
	if providerCfg.Model != "deepseek-reasoner" || providerCfg.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Fatalf("unexpected selected provider config %#v", providerCfg)
	}
}

func TestSelectProviderRuntimeModelRejectsUnknownProvider(t *testing.T) {
	if _, _, err := SelectProviderRuntimeModel(ProviderRuntimeConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Type: "openai-compatible", Model: "gpt-5.4"},
		},
	}, "missing", "gpt-5.4"); err == nil {
		t.Fatal("expected missing provider selection to fail")
	}
}

func TestDeleteProviderRuntimeProviderRemovesTargetAndPreservesCurrentDefault(t *testing.T) {
	runtime, providerCfg, err := DeleteProviderRuntimeProvider(ProviderRuntimeConfig{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-5.4-mini",
		Providers: map[string]ProviderConfig{
			"openai":   {Type: "openai-compatible", Model: "gpt-5.4-mini", APIKeyEnv: "OPENAI_API_KEY"},
			"deepseek": {Type: "openai-compatible", Model: "deepseek-chat", APIKeyEnv: "DEEPSEEK_API_KEY"},
		},
	}, "deepseek")
	if err != nil {
		t.Fatalf("expected runtime deletion to succeed, got %v", err)
	}
	if runtime.DefaultProvider != "openai" || runtime.DefaultModel != "gpt-5.4-mini" {
		t.Fatalf("unexpected runtime defaults %#v", runtime)
	}
	if _, ok := runtime.Providers["deepseek"]; ok {
		t.Fatalf("expected deepseek provider to be removed, got %#v", runtime.Providers)
	}
	if providerCfg.Model != "gpt-5.4-mini" || providerCfg.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("unexpected surviving provider config %#v", providerCfg)
	}
}

func TestDeleteProviderRuntimeProviderReselectsDefaultWhenActiveTargetDeleted(t *testing.T) {
	runtime, providerCfg, err := DeleteProviderRuntimeProvider(ProviderRuntimeConfig{
		DefaultProvider: "deepseek",
		DefaultModel:    "deepseek-reasoner",
		Providers: map[string]ProviderConfig{
			"openai-primary": {Type: "openai-compatible", Model: "gpt-5.4-mini", APIKeyEnv: "OPENAI_API_KEY"},
			"deepseek":       {Type: "openai-compatible", Model: "deepseek-reasoner", APIKeyEnv: "DEEPSEEK_API_KEY"},
			"zai":            {Type: "openai-compatible", Model: "glm-4.6", APIKeyEnv: "ZAI_API_KEY"},
		},
	}, "deepseek")
	if err != nil {
		t.Fatalf("expected runtime deletion to succeed, got %v", err)
	}
	if runtime.DefaultProvider != "openai-primary" || runtime.DefaultModel != "gpt-5.4-mini" {
		t.Fatalf("expected lexicographically first remaining provider to become default, got %#v", runtime)
	}
	if providerCfg.Model != "gpt-5.4-mini" || providerCfg.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("unexpected selected provider config %#v", providerCfg)
	}
}

func TestDeleteProviderRuntimeProviderRejectsDeletingLastConfiguredModel(t *testing.T) {
	if _, _, err := DeleteProviderRuntimeProvider(ProviderRuntimeConfig{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-5.4",
		Providers: map[string]ProviderConfig{
			"openai": {Type: "openai-compatible", Model: "gpt-5.4"},
		},
	}, "openai"); err == nil {
		t.Fatal("expected deleting the last configured provider to fail")
	}
}

func TestConfigLoadRejectsDuplicateNormalizedProviderRuntimeIDs(t *testing.T) {
	workspace := t.TempDir()
	writeProviderRuntimeConfigFile(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"provider_runtime": map[string]any{
			"default_provider": "openai",
			"default_model":    "gpt-5.4-mini",
			"providers": map[string]any{
				"OpenAI": map[string]any{
					"type":     "openai-compatible",
					"base_url": "https://api.openai.com/v1",
					"model":    "gpt-5.4-mini",
				},
				"openai": map[string]any{
					"type":     "openai-compatible",
					"base_url": "https://api.openai.com/v1",
					"model":    "gpt-5.4-mini",
				},
			},
		},
	})

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected duplicate provider id error")
	}
	if !strings.Contains(err.Error(), "duplicate provider id after normalization") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigLoadPreservesExplicitProviderRuntimeFieldsWhenProvidersMissing(t *testing.T) {
	workspace := t.TempDir()
	writeProviderRuntimeConfigFile(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "legacy-model",
			"api_key":  "test-key",
		},
		"provider_runtime": map[string]any{
			"default_provider": "anthropic",
			"default_model":    "runtime-model",
			"allow_fallback":   true,
		},
	})

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderRuntime.DefaultProvider != "anthropic" {
		t.Fatalf("expected explicit default_provider to be preserved, got %q", cfg.ProviderRuntime.DefaultProvider)
	}
	if cfg.ProviderRuntime.DefaultModel != "runtime-model" {
		t.Fatalf("expected explicit default_model to be preserved, got %q", cfg.ProviderRuntime.DefaultModel)
	}
	if !cfg.ProviderRuntime.AllowFallback {
		t.Fatalf("expected explicit allow_fallback to be preserved, got %#v", cfg.ProviderRuntime)
	}
	if len(cfg.ProviderRuntime.Providers) != 1 {
		t.Fatalf("expected one legacy provider fallback entry, got %#v", cfg.ProviderRuntime.Providers)
	}
	if _, ok := cfg.ProviderRuntime.Providers["openai"]; !ok {
		t.Fatalf("expected legacy provider entry to be backfilled, got %#v", cfg.ProviderRuntime.Providers)
	}
}
