package tui

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"bytemind/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchCurrentMonthUsageValidation(t *testing.T) {
	_, err := fetchCurrentMonthUsage(config.Config{})
	if err == nil || !strings.Contains(err.Error(), "missing base url or api key") {
		t.Fatalf("expected missing-base-or-key validation error, got %v", err)
	}

	cfg := config.Config{
		Provider: config.ProviderConfig{
			BaseURL: "https://example.com/v1",
			APIKey:  "test-key",
		},
	}
	_, err = fetchCurrentMonthUsage(cfg)
	if err == nil || !strings.Contains(err.Error(), "non-openai base url") {
		t.Fatalf("expected non-openai validation error, got %v", err)
	}
}

func TestFetchCurrentMonthUsageHTTPErrorAndDecodeError(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	cfg := config.Config{
		Provider: config.ProviderConfig{
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "test-key",
		},
	}

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})
	_, err := fetchCurrentMonthUsage(cfg)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected network error to propagate, got %v", err)
	}

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("bad request")),
			Header:     make(http.Header),
		}, nil
	})
	_, err = fetchCurrentMonthUsage(cfg)
	if err == nil || !strings.Contains(err.Error(), "usage pull failed (400): bad request") {
		t.Fatalf("expected status error, got %v", err)
	}

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("{not-json")),
			Header:     make(http.Header),
		}, nil
	})
	_, err = fetchCurrentMonthUsage(cfg)
	if err == nil {
		t.Fatalf("expected json decode error")
	}
}

func TestFetchCurrentMonthUsageSuccess(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	cfg := config.Config{
		Provider: config.ProviderConfig{
			BaseURL: "https://api.openai.com",
			APIKey:  "test-key",
		},
	}

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", req.Method)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected auth header, got %q", got)
		}
		if !strings.Contains(req.URL.Path, "/v1/organization/usage/completions") {
			t.Fatalf("expected usage endpoint path, got %q", req.URL.Path)
		}
		if req.URL.Query().Get("bucket_width") != "1d" {
			t.Fatalf("expected bucket_width=1d, got %q", req.URL.Query().Get("bucket_width"))
		}
		body := `{
  "data": [
    {
      "results": [
        {
          "input_tokens": 10,
          "output_tokens": 5,
          "input_cached_tokens": 3,
          "input_audio_tokens": 2,
          "output_audio_tokens": 1
        },
        {
          "input_tokens": -10,
          "output_tokens": -2,
          "input_cached_tokens": -1,
          "input_audio_tokens": 0,
          "output_audio_tokens": 0
        }
      ]
    }
  ]
}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	usage, err := fetchCurrentMonthUsage(cfg)
	if err != nil {
		t.Fatalf("expected successful usage pull, got %v", err)
	}
	if usage.Input != 12 {
		t.Fatalf("expected input tokens 12, got %d", usage.Input)
	}
	if usage.Output != 6 {
		t.Fatalf("expected output tokens 6, got %d", usage.Output)
	}
	if usage.Context != 3 {
		t.Fatalf("expected cached/context tokens 3, got %d", usage.Context)
	}
	if usage.Used != 21 {
		t.Fatalf("expected total used tokens 21, got %d", usage.Used)
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	if got := normalizeOpenAIBaseURL(" https://api.openai.com/v1/ "); got != "https://api.openai.com/v1" {
		t.Fatalf("expected to preserve /v1 suffix, got %q", got)
	}
	if got := normalizeOpenAIBaseURL("https://api.openai.com"); got != "https://api.openai.com/v1" {
		t.Fatalf("expected /v1 suffix to be appended, got %q", got)
	}
}

