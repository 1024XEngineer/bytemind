package provider

import (
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
