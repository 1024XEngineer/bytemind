package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bytemind/internal/config"
)

func TestCheckAvailabilityReturnsMissingKey(t *testing.T) {
	t.Setenv("BYTEMIND_API_KEY", "")

	result := CheckAvailability(context.Background(), config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-5.4",
		APIKey:  "",
	})

	if result.Ready {
		t.Fatal("expected unavailable result when api key is missing")
	}
	if !strings.Contains(result.Reason, "missing API key") {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
}

func TestCheckAvailabilityReturnsReadyOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckAvailability(context.Background(), config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: server.URL,
		Model:   "gpt-5.4",
		APIKey:  "test-key",
	})

	if !result.Ready {
		t.Fatalf("expected ready=true, got %+v", result)
	}
}

func TestCheckAvailabilityReturnsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_api_key"}`))
	}))
	defer server.Close()

	result := CheckAvailability(context.Background(), config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: server.URL,
		Model:   "gpt-5.4",
		APIKey:  "bad-key",
	})

	if result.Ready {
		t.Fatal("expected unavailable result for unauthorized key")
	}
	if !strings.Contains(result.Reason, "unauthorized") {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
	if !strings.Contains(result.Detail, "invalid_api_key") {
		t.Fatalf("unexpected detail: %q", result.Detail)
	}
}

func TestCheckAvailabilityUsesAnthropicHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "anth-key" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			http.Error(w, "bad version", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckAvailability(context.Background(), config.ProviderConfig{
		Type:    "anthropic",
		BaseURL: server.URL,
		Model:   "claude",
		APIKey:  "anth-key",
	})

	if !result.Ready {
		t.Fatalf("expected ready=true, got %+v", result)
	}
}

func TestCheckAvailabilityUsesGeminiHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gem-key" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckAvailability(context.Background(), config.ProviderConfig{
		Type:    "gemini",
		BaseURL: server.URL,
		Model:   "gemini-2.5-flash",
		APIKey:  "gem-key",
	})

	if !result.Ready {
		t.Fatalf("expected ready=true, got %+v", result)
	}
}
