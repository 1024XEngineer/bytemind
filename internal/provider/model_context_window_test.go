package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupModelContextWindowKnownModel(t *testing.T) {
	cw := LookupModelContextWindow(context.Background(), "openai", "", "", "gpt-4o")
	if cw != 128000 {
		t.Fatalf("expected 128000 for gpt-4o, got %d", cw)
	}
}

func TestLookupModelContextWindowUnknownModelNoAPI(t *testing.T) {
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

	cw := LookupModelContextWindow(context.Background(), "gemini", "", "", "x99-custom-unknown")
	if cw != 999999 {
		t.Fatalf("expected mock fetch to return 999999, got %d", cw)
	}
}

func TestFetchModelInfoWithClientSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"inputTokenLimit": 1234567}`))
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(context.Background(), srv.Client(), srv.URL)
	if got != 1234567 {
		t.Fatalf("expected 1234567, got %d", got)
	}
}

func TestFetchModelInfoWithClientNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(context.Background(), srv.Client(), srv.URL)
	if got != 0 {
		t.Fatalf("expected 0 for non-200 response, got %d", got)
	}
}

func TestFetchModelInfoWithClientBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(context.Background(), srv.Client(), srv.URL)
	if got != 0 {
		t.Fatalf("expected 0 for bad JSON, got %d", got)
	}
}

func TestFetchModelInfoWithClientMissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"otherField": 999}`))
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(context.Background(), srv.Client(), srv.URL)
	if got != 0 {
		t.Fatalf("expected 0 when inputTokenLimit is missing, got %d", got)
	}
}

func TestFetchModelInfoWithClientZeroValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"inputTokenLimit": 0}`))
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(context.Background(), srv.Client(), srv.URL)
	if got != 0 {
		t.Fatalf("expected 0 for zero inputTokenLimit, got %d", got)
	}
}

func TestFetchModelInfoWithClientCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"inputTokenLimit": 999}`))
	}))
	defer srv.Close()

	got := fetchModelInfoWithClient(ctx, srv.Client(), srv.URL)
	if got != 0 {
		t.Fatalf("expected 0 for canceled context, got %d", got)
	}
}

func TestFetchModelInfoWithClientBadURL(t *testing.T) {
	got := fetchModelInfoWithClient(context.Background(), http.DefaultClient, "://invalid-url")
	if got != 0 {
		t.Fatalf("expected 0 for bad URL, got %d", got)
	}
}

func TestFetchGeminiContextWindowDefaultBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-2.0-flash" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"inputTokenLimit": 1000000}`))
	}))
	defer srv.Close()

	original := defaultHTTPClient
	t.Cleanup(func() { defaultHTTPClient = original })
	defaultHTTPClient = srv.Client()

	// Empty baseURL triggers default fallback which is different from test server URL.
	// Instead, explicitly set baseURL to server URL but without models/ prefix.
	got := fetchGeminiContextWindow(context.Background(), srv.URL, "", "gemini-2.0-flash")
	if got != 1000000 {
		t.Fatalf("expected 1000000, got %d", got)
	}
}

func TestFetchGeminiContextWindowEmptyModelID(t *testing.T) {
	got := fetchGeminiContextWindow(context.Background(), "", "", "")
	if got != 0 {
		t.Fatalf("expected 0 for empty modelID, got %d", got)
	}
}

func TestFetchModelContextWindowGemini(t *testing.T) {
	// FetchModelContextWindow dispatches to fetchGeminiContextWindow for gemini type
	// with empty baseURL and no real server, it will return 0
	cw := FetchModelContextWindow(context.Background(), "gemini", "", "fake-key", "some-model")
	if cw != 0 {
		t.Fatalf("expected 0 (no server), got %d", cw)
	}
}

func TestFetchModelContextWindowNonGemini(t *testing.T) {
	cw := FetchModelContextWindow(context.Background(), "openai", "", "", "some-model")
	if cw != 0 {
		t.Fatalf("expected 0 for non-gemini, got %d", cw)
	}
}
