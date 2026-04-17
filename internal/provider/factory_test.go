package provider

import (
	"context"
	"strings"
	"testing"

	"bytemind/internal/config"
)

func TestNewClientReturnsOpenAICompatible(t *testing.T) {
	client, err := NewClient(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := client.(*OpenAICompatible); !ok {
		t.Fatalf("expected *OpenAICompatible, got %T", client)
	}
}

func TestNewBaseClientReturnsOpenAICompatible(t *testing.T) {
	client, err := newBaseClient(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := client.(*OpenAICompatible); !ok {
		t.Fatalf("expected *OpenAICompatible, got %T", client)
	}
}

func TestNewClientPropagatesOpenAICompatibleGatewayConfig(t *testing.T) {
	client, err := newBaseClient(config.ProviderConfig{
		Type:         "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		APIPath:      "/gateway/chat",
		APIKey:       "test-key",
		AuthHeader:   "X-API-Key",
		AuthScheme:   "Token",
		ExtraHeaders: map[string]string{"X-Gateway": "enabled"},
		Model:        "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	openaiClient, ok := client.(*OpenAICompatible)
	if !ok {
		t.Fatalf("expected *OpenAICompatible, got %T", client)
	}
	if openaiClient.apiPath != "/gateway/chat" || openaiClient.authHeader != "X-API-Key" || openaiClient.authScheme != "Token" || openaiClient.extraHeaders["X-Gateway"] != "enabled" {
		t.Fatalf("unexpected openai gateway config %#v", openaiClient)
	}
}

func TestNewBaseClientReturnsOpenAICompatibleForOpenAIAlias(t *testing.T) {
	client, err := newBaseClient(config.ProviderConfig{
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := client.(*OpenAICompatible); !ok {
		t.Fatalf("expected *OpenAICompatible, got %T", client)
	}
}

func TestNewBaseClientReturnsAnthropic(t *testing.T) {
	client, err := newBaseClient(config.ProviderConfig{
		Type:             "anthropic",
		BaseURL:          "https://api.anthropic.com",
		APIKey:           "test-key",
		Model:            "claude-sonnet",
		AnthropicVersion: "2023-06-01",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := client.(*Anthropic); !ok {
		t.Fatalf("expected *Anthropic, got %T", client)
	}
}

func TestNewDomainClientWrapsBaseClient(t *testing.T) {
	client, err := NewDomainClient(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client.ProviderID() != ProviderOpenAI {
		t.Fatalf("expected provider id %q, got %q", ProviderOpenAI, client.ProviderID())
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("expected no error listing models, got %v", err)
	}
	if len(models) != 1 || models[0].ModelID != ModelID("gpt-5.4") {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestNewBaseClientRejectsUnsupportedProviderType(t *testing.T) {
	_, err := newBaseClient(config.ProviderConfig{
		Type:    "unsupported",
		BaseURL: "https://example.com",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err == nil {
		t.Fatal("expected unsupported provider type error")
	}
	if !strings.Contains(err.Error(), "unsupported provider type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDomainClientWithIDUsesExplicitProviderInstanceID(t *testing.T) {
	clientA, err := NewDomainClientWithID("provider-a", config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	clientB, err := NewDomainClientWithID("provider-b", config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://example.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-4.1",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if clientA.ProviderID() != "provider-a" || clientB.ProviderID() != "provider-b" {
		t.Fatalf("unexpected provider ids %q %q", clientA.ProviderID(), clientB.ProviderID())
	}
}

func TestNewDomainClientPreservesAnthropicProviderID(t *testing.T) {
	client, err := NewDomainClient(config.ProviderConfig{
		Type:             "anthropic",
		BaseURL:          "https://api.anthropic.com",
		APIKey:           "test-key",
		Model:            "claude-sonnet",
		AnthropicVersion: "2023-06-01",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client.ProviderID() != ProviderAnthropic {
		t.Fatalf("expected provider id %q, got %q", ProviderAnthropic, client.ProviderID())
	}
}

func TestNewDomainClientNormalizesOpenAIProviderIDVariants(t *testing.T) {
	client, err := NewDomainClient(config.ProviderConfig{
		Type:    " OpenAI-Compatible ",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client.ProviderID() != ProviderOpenAI {
		t.Fatalf("expected provider id %q, got %q", ProviderOpenAI, client.ProviderID())
	}
}

func TestNewBaseClientAcceptsNormalizedTypeVariants(t *testing.T) {
	cases := []config.ProviderConfig{
		{Type: " OPENAI ", BaseURL: "https://api.openai.com/v1", APIKey: "test-key", Model: "gpt-5.4"},
		{Type: "OpenAI-Compatible", BaseURL: "https://api.openai.com/v1", APIKey: "test-key", Model: "gpt-5.4"},
		{Type: " ANTHROPIC ", BaseURL: "https://api.anthropic.com", APIKey: "test-key", Model: "claude-sonnet", AnthropicVersion: "2023-06-01"},
	}
	for _, cfg := range cases {
		if _, err := newBaseClient(cfg); err != nil {
			t.Fatalf("expected normalized type %q to succeed, got %v", cfg.Type, err)
		}
	}
}

func TestNewClientFromRuntimeUsesRouterFacade(t *testing.T) {
	client, err := NewClientFromRuntime(config.LegacyProviderRuntimeConfig(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	}), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := client.(*RoutedClient); !ok {
		t.Fatalf("expected routed client, got %T", client)
	}
}

func TestLegacyRuntimeConfigWithEmptyTypeBuildsRegistry(t *testing.T) {
	runtime := config.LegacyProviderRuntimeConfig(config.ProviderConfig{
		Type:    "",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	reg, err := NewRegistry(runtime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	client, ok := reg.Get(context.Background(), "openai")
	if !ok || client == nil {
		t.Fatalf("expected openai client from legacy runtime config, got %#v ok=%v", client, ok)
	}
}

func TestNewRouterClient(t *testing.T) {
	client, err := NewRouterClient(config.ProviderRuntimeConfig{DefaultProvider: "openai", DefaultModel: "gpt-5.4", AllowFallback: true, Providers: map[string]config.ProviderConfig{"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", APIKey: "key", Model: "gpt-5.4"}}}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	routed, ok := client.(*RoutedClient)
	if !ok {
		t.Fatalf("expected routed client, got %T", client)
	}
	if !routed.allowFallback {
		t.Fatal("expected routed client fallback to be enabled")
	}
	if routed.health == nil {
		t.Fatal("expected default health checker to be wired")
	}
	if _, err := NewRouterClient(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{"broken": {Type: ""}}}, nil); err == nil {
		t.Fatal("expected registry error")
	}
}

func TestHealthConfigFromRuntime(t *testing.T) {
	cfg := HealthConfigFromRuntime(config.ProviderHealthRuntimeConfig{FailThreshold: 4, RecoverProbeSec: 12, RecoverSuccessThreshold: 3, WindowSize: 6})
	if cfg.FailThreshold != 4 || cfg.RecoverProbeSec != 12 || cfg.RecoverSuccessThreshold != 3 || cfg.WindowSize != 6 {
		t.Fatalf("unexpected health config %#v", cfg)
	}
}

func TestNewDomainClientRejectsEmptyType(t *testing.T) {
	client, err := NewDomainClient(config.ProviderConfig{
		Type:    "",
		BaseURL: "https://example.com",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err == nil {
		t.Fatal("expected unsupported provider type error")
	}
	if client != nil {
		t.Fatalf("expected nil client, got %v", client)
	}
}
