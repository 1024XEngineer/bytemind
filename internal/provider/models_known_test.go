package provider

import (
	"testing"
)

func TestNormalizeModelID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gpt-4o", "gpt-4o"},
		{"GPT-4o", "gpt-4o"},
		{"GPT-4O", "gpt-4o"},
		{"azure/gpt-4o", "gpt-4o"},
		{"openai/gpt-5.4", "gpt-5.4"},
		{"aws-bedrock/llama-4-scout", "llama-4-scout"},
		{" deepseek/deepseek-chat ", "deepseek-chat"},
		// "meta-llama" is not a known provider prefix, so path is preserved
		// knownContextWindow will still match via the tail-after-last-/ fallback
		{"mistral/mistral-large-2512", "mistral-large-2512"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"", ""},
		{"  ", ""},
	}
	for _, tc := range tests {
		got := normalizeModelID(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeModelID(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestKnownContextWindow(t *testing.T) {
	tests := []struct {
		modelID  string
		expected int
	}{
		// Exact match
		{"gpt-4-32k", 32768},
		{"gpt-4-turbo", 128000},
		// Prefix match — standard
		{"gpt-4o", 128000},
		{"gpt-4o-2024-08-06", 128000},
		{"gpt-5.4", 1000000},
		{"gpt-5.4-mini", 400000},
		{"gpt-5.5", 1000000},
		{"gpt-4.1", 1047576},
		{"o1-preview", 200000},
		// Case insensitive
		{"GPT-4O", 128000},
		{"Claude-Sonnet-4-6", 1000000},
		{"DEEPSEEK-V4-FLASH", 1000000},
		// Provider prefix stripping
		{"azure/gpt-4o", 128000},
		{"openai/gpt-5.4", 1000000},
		{"aws-bedrock/llama-4-scout", 10000000},
		{"deepseek/deepseek-chat", 1000000},
		{"google/gemini-2.5-pro", 1048576},
		// Path-style ID — match after last /
		{"meta-llama/Llama-4-Scout-17B", 10000000},
		// Anthropic
		{"claude-opus-4-7", 1000000},
		{"claude-sonnet-4-6", 1000000},
		{"claude-haiku-4-5", 200000},
		{"claude-4", 200000},
		// Gemini
		{"gemini-3.1-pro-preview", 1048576},
		{"gemini-2.0-flash", 1000000},
		{"gemini-1.5-pro", 2000000},
		// DeepSeek
		{"deepseek-v4-flash", 1000000},
		{"deepseek-v4-pro", 1000000},
		{"deepseek-chat", 1000000},
		{"deepseek-reasoner", 1000000},
		{"deepseek-v3", 65536},
		{"deepseek-r1", 65536},
		// Qwen
		{"qwen-long-latest", 10000000},
		{"qwen3-max", 262144},
		{"qwen-plus", 1000000},
		{"qwen-turbo-latest", 1000000},
		// Mistral
		{"mistral-large-2512", 256000},
		{"mistral-medium-3-5", 256000},
		{"mistral-small-2506", 128000},
		// xAI
		{"grok-4.3", 1000000},
		// Llama
		{"llama-4-scout", 10000000},
		{"llama-4-maverick", 1000000},
		// Cohere
		{"command-a-03-2025", 256000},
		{"command-r-08-2024", 128000},
		// Z.AI / GLM
		{"glm-4.5", 128000},
		// Kimi
		{"kimi-k2.6", 256000},
		{"kimi-k2.5", 256000},
		// Unknown model returns 0
		{"totally-unknown-model-9000", 0},
		{"", 0},
	}
	for _, tc := range tests {
		got := knownContextWindow(tc.modelID)
		if got != tc.expected {
			t.Errorf("knownContextWindow(%q) = %d, want %d", tc.modelID, got, tc.expected)
		}
	}
}

func TestIsProviderPrefix(t *testing.T) {
	if !isProviderPrefix("openai") {
		t.Error("expected openai to be a provider prefix")
	}
	if !isProviderPrefix("azure") {
		t.Error("expected azure to be a provider prefix")
	}
	if !isProviderPrefix("aws-bedrock") {
		t.Error("expected aws-bedrock to be a provider prefix")
	}
	if !isProviderPrefix("huggingface") {
		t.Error("expected huggingface to be a provider prefix")
	}
	if isProviderPrefix("my-custom-service") {
		t.Error("expected my-custom-service to NOT be a provider prefix")
	}
	if isProviderPrefix("") {
		t.Error("expected empty string to NOT be a provider prefix")
	}
}

func TestFillContextWindow(t *testing.T) {
	meta := map[string]string{}
	fillContextWindow(meta, "gpt-4o")
	if meta["context_window"] != "128000" {
		t.Fatalf("expected context_window=128000, got %q", meta["context_window"])
	}

	meta2 := map[string]string{"context_window": "8888"}
	fillContextWindow(meta2, "gpt-4o")
	if meta2["context_window"] != "8888" {
		t.Fatalf("expected context_window to remain 8888, got %q", meta2["context_window"])
	}

	meta3 := map[string]string{}
	fillContextWindow(meta3, "unknown-model")
	if meta3["context_window"] != "" {
		t.Fatalf("expected context_window to remain empty for unknown model, got %q", meta3["context_window"])
	}

	meta4 := map[string]string{}
	fillContextWindow(meta4, "")
	if meta4["context_window"] != "" {
		t.Fatalf("expected context_window to remain empty for empty modelID, got %q", meta4["context_window"])
	}
}

func TestNormalizeModelIDPreservesUnknownPrefix(t *testing.T) {
	// "org/" that isn't a known provider prefix should not be stripped
	got := normalizeModelID("mycompany/gpt-4o")
	// "mycompany" is not a known provider prefix, so it should remain
	if got != "mycompany/gpt-4o" {
		t.Errorf("expected %q, got %q", "mycompany/gpt-4o", got)
	}
}
