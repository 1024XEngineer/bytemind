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

func TestNewClientReturnsOpenAICompatibleForOpenAIAlias(t *testing.T) {
	client, err := NewClient(config.ProviderConfig{
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

func TestNewClientReturnsAnthropic(t *testing.T) {
	client, err := NewClient(config.ProviderConfig{
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

func TestNewClientRejectsUnsupportedProviderType(t *testing.T) {
	_, err := NewClient(config.ProviderConfig{
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

func TestNewClientAcceptsNormalizedTypeVariants(t *testing.T) {
	cases := []config.ProviderConfig{
		{Type: " OPENAI ", BaseURL: "https://api.openai.com/v1", APIKey: "test-key", Model: "gpt-5.4"},
		{Type: "OpenAI-Compatible", BaseURL: "https://api.openai.com/v1", APIKey: "test-key", Model: "gpt-5.4"},
		{Type: " ANTHROPIC ", BaseURL: "https://api.anthropic.com", APIKey: "test-key", Model: "claude-sonnet", AnthropicVersion: "2023-06-01"},
	}
	for _, cfg := range cases {
		if _, err := NewClient(cfg); err != nil {
			t.Fatalf("expected normalized type %q to succeed, got %v", cfg.Type, err)
		}
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
