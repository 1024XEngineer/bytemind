package tools

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
)

func TestWebSearchToolReturnsNormalizedResultsFromRSS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.URL.Query().Get("q"); got != "golang generics" {
			t.Fatalf("unexpected query %q", got)
		}
		if got := r.URL.Query().Get("format"); got != "rss" {
			t.Fatalf("unexpected format %q", got)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>The Go Programming Language</title>
      <link>https://go.dev/</link>
      <description>An open source programming language.</description>
    </item>
    <item>
      <title>Tutorial: Getting started with generics in Go</title>
      <link>https://go.dev/doc/tutorial/generics</link>
      <description>Type parameters in Go.</description>
    </item>
    <item>
      <title>Duplicate</title>
      <link>https://go.dev/</link>
      <description>Duplicate result should be removed.</description>
    </item>
  </channel>
</rss>`))
	}))
	defer server.Close()

	tool := WebSearchTool{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"query":"golang generics","max_results":2}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK      bool `json:"ok"`
		Query   string
		Source  string
		Results []struct {
			Title string
			URL   string
			Rank  int
		}
	}
	decodeJSONResult(t, result, &payload)

	if !payload.OK {
		t.Fatalf("expected ok=true, got %+v", payload)
	}
	if payload.Query != "golang generics" {
		t.Fatalf("unexpected query metadata %+v", payload)
	}
	if !strings.Contains(payload.Source, "127.0.0.1") && !strings.Contains(payload.Source, "localhost") {
		t.Fatalf("unexpected source %q", payload.Source)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results, got %+v", payload.Results)
	}
	if payload.Results[0].URL != "https://go.dev/" || payload.Results[0].Rank != 1 {
		t.Fatalf("unexpected first result %+v", payload.Results[0])
	}
	if payload.Results[1].Rank != 2 {
		t.Fatalf("expected second rank=2, got %+v", payload.Results[1])
	}
}

func TestWebSearchToolFallsBackToHTMLAnchors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
<a class="result__a" href="https://example.com/a">Result A</a>
<a class="result__a" href="https://example.com/b">Result B</a>
</body></html>`))
	}))
	defer server.Close()

	tool := WebSearchTool{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"query":"sample","max_results":2}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK      bool `json:"ok"`
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	decodeJSONResult(t, result, &payload)
	if !payload.OK || len(payload.Results) != 2 {
		t.Fatalf("unexpected html fallback payload %+v", payload)
	}
	if payload.Results[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected first result %+v", payload.Results[0])
	}
}

func TestWebSearchToolReturnsProviderErrorPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	tool := WebSearchTool{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"query":"latest ai news"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
		Status    int    `json:"status_code"`
	}
	decodeJSONResult(t, result, &payload)

	if payload.OK || payload.ErrorCode != "upstream_rate_limited" || payload.Status != http.StatusTooManyRequests {
		t.Fatalf("unexpected provider error payload %+v", payload)
	}
}

func TestWebSearchToolReturnsNetworkUnreachablePayload(t *testing.T) {
	tool := WebSearchTool{
		BaseURL: "https://example.com/search",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, &neturl.Error{
					Op:  "Get",
					URL: req.URL.String(),
					Err: errors.New("dial tcp: network unreachable"),
				}
			}),
		},
	}

	result, err := tool.Run(context.Background(), []byte(`{"query":"breaking market news"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
	}
	decodeJSONResult(t, result, &payload)

	if payload.OK || payload.ErrorCode != "network_unreachable" {
		t.Fatalf("unexpected network payload %+v", payload)
	}
}
