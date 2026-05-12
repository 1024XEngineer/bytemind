package provider

import (
	"context"
	"testing"
)

func TestLookupModelContextWindowKnownModel(t *testing.T) {
	cw := LookupModelContextWindow(context.Background(), "openai", "", "", "gpt-4o")
	if cw != 128000 {
		t.Fatalf("expected 128000 for gpt-4o, got %d", cw)
	}
}

func TestLookupModelContextWindowUnknownModelNoAPI(t *testing.T) {
	// Non-Gemini provider should not attempt API call
	cw := LookupModelContextWindow(context.Background(), "openai", "", "", "totally-unknown-model")
	if cw != 0 {
		t.Fatalf("expected 0 for unknown model, got %d", cw)
	}
}

func TestLookupModelContextWindowNilContext(t *testing.T) {
	cw := LookupModelContextWindow(nil, "gemini", "", "", "totally-unknown-model")
	if cw != 0 {
		t.Fatalf("expected 0 with nil context, got %d", cw)
	}
}

func TestLookupModelContextWindowGeminiUsesFetch(t *testing.T) {
	original := contextWindowFetchFunc
	t.Cleanup(func() { contextWindowFetchFunc = original })

	contextWindowFetchFunc = func(_ context.Context, providerType, _, _, _ string) int {
		if providerType == "gemini" {
			return 999999
		}
		return 0
	}

	cw := LookupModelContextWindow(context.Background(), "gemini", "", "", "gemini-unknown-v42")
	if cw != 999999 {
		t.Fatalf("expected mock fetch to return 999999, got %d", cw)
	}
}
