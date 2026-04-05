package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWebSearchParsersAndHelpers(t *testing.T) {
	if got := parseRSSResults([]byte("not xml"), 3); got != nil {
		t.Fatalf("expected invalid xml => nil, got %+v", got)
	}
	if got := parseRSSResults([]byte(`<feed><entry>x</entry></feed>`), 3); got != nil {
		t.Fatalf("expected non-rss root => nil, got %+v", got)
	}

	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item><title></title><link>https://example.com/a</link><description>desc <b>a</b></description></item>
    <item><title>B</title><link>https://example.com/b</link><description>desc b</description></item>
    <item><title>dup</title><link>https://example.com/a</link><description>dup</description></item>
  </channel>
</rss>`)
	rssResults := parseRSSResults(rss, 10)
	if len(rssResults) != 2 {
		t.Fatalf("expected deduped rss results, got %+v", rssResults)
	}
	if rssResults[0].Title != "example.com" || rssResults[0].Rank != 1 {
		t.Fatalf("unexpected first rss result %+v", rssResults[0])
	}
	if rssResults[1].Title != "B" || rssResults[1].Rank != 2 {
		t.Fatalf("unexpected second rss result %+v", rssResults[1])
	}

	endpoint, err := url.Parse("https://search.example.com/search?q=go")
	if err != nil {
		t.Fatal(err)
	}
	htmlPayload := []byte(`<html><body>
<a href="/relative">Relative Result</a>
<a href="https://example.com/cached">Cached page</a>
<a href="https://example.com/similar">Similar links</a>
<a href="https://example.com/final">Final Result</a>
</body></html>`)
	htmlResults := parseHTMLAnchorResults(htmlPayload, endpoint, 5)
	if len(htmlResults) != 2 {
		t.Fatalf("expected filtered html results, got %+v", htmlResults)
	}
	if htmlResults[0].URL != "https://search.example.com/relative" {
		t.Fatalf("expected resolved relative URL, got %+v", htmlResults[0])
	}
	if htmlResults[1].URL != "https://example.com/final" {
		t.Fatalf("unexpected second URL %+v", htmlResults[1])
	}

	redirectPayload := []byte(`<a class="result__a" href="https://search.example.com/l/?uddg=https%3A%2F%2Fgo.dev%2Fdoc">Go docs</a>`)
	redirectResults := parseHTMLAnchorResults(redirectPayload, endpoint, 2)
	if len(redirectResults) != 1 || redirectResults[0].URL != "https://go.dev/doc" {
		t.Fatalf("expected redirect target extraction, got %+v", redirectResults)
	}

	if got := normalizeSearchLink("//example.com/path", endpoint); got != "https://example.com/path" {
		t.Fatalf("unexpected protocol-relative normalization %q", got)
	}
	if got := normalizeSearchLink("/docs", endpoint); got != "https://search.example.com/docs" {
		t.Fatalf("unexpected relative normalization %q", got)
	}
	if got := normalizeSearchLink("mailto:test@example.com", endpoint); got != "" {
		t.Fatalf("expected non-http URL rejection, got %q", got)
	}
	if got, ok := normalizeAbsoluteHTTPURL("ftp://example.com"); ok || got != "" {
		t.Fatalf("expected ftp URL rejection, got=%q ok=%v", got, ok)
	}

	if got := fallbackSearchTitle("  <b>Hello</b>  ", "https://example.com", "snippet"); got != "Hello" {
		t.Fatalf("unexpected title fallback %q", got)
	}
	if got := fallbackSearchTitle("", "https://go.dev/doc", "snippet"); got != "go.dev" {
		t.Fatalf("expected host fallback, got %q", got)
	}
	if got := fallbackSearchTitle("", "not-a-url", strings.Repeat("x", 90)); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated snippet fallback, got %q", got)
	}
	if got := cleanSearchSnippet("  A&nbsp;<b>bold</b>\nline  "); got != "A bold line" {
		t.Fatalf("unexpected cleaned snippet %q", got)
	}
	if looksSearchResultTitle("cached item") || looksSearchResultTitle("similar links") {
		t.Fatal("expected cached/similar title to be filtered")
	}
	if !looksSearchResultTitle("normal title") {
		t.Fatal("expected normal title")
	}
}

func TestWebSearchRunValidationAndLimits(t *testing.T) {
	tool := WebSearchTool{}
	if _, err := tool.Run(context.Background(), []byte(`{"query":"  "}`), &ExecutionContext{}); err == nil {
		t.Fatal("expected query validation error")
	}

	invalid := WebSearchTool{BaseURL: "://invalid"}
	result, err := invalid.Run(context.Background(), []byte(`{"query":"golang"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var invalidPayload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
	}
	decodeJSONResult(t, result, &invalidPayload)
	if invalidPayload.OK || invalidPayload.ErrorCode != "invalid_configuration" {
		t.Fatalf("unexpected invalid config payload %+v", invalidPayload)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("format"); got != "atom" {
			t.Fatalf("expected explicit format preserved, got %q", got)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(buildSearchRSS(16)))
	}))
	defer server.Close()

	tool = WebSearchTool{
		BaseURL:    server.URL + "?format=atom",
		HTTPClient: server.Client(),
	}
	result, err = tool.Run(context.Background(), []byte(`{"query":"golang","max_results":999}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK      bool `json:"ok"`
		Results []struct {
			Rank int `json:"rank"`
		} `json:"results"`
	}
	decodeJSONResult(t, result, &payload)
	if !payload.OK {
		t.Fatalf("unexpected payload %+v", payload)
	}
	if len(payload.Results) != maxWebSearchLimit {
		t.Fatalf("expected clamped results=%d, got %d", maxWebSearchLimit, len(payload.Results))
	}
	if payload.Results[0].Rank != 1 || payload.Results[len(payload.Results)-1].Rank != maxWebSearchLimit {
		t.Fatalf("expected contiguous ranks, got first=%d last=%d", payload.Results[0].Rank, payload.Results[len(payload.Results)-1].Rank)
	}
}

func buildSearchRSS(count int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
	for i := 1; i <= count; i++ {
		b.WriteString(fmt.Sprintf(`<item><title>Result %d</title><link>https://example.com/%d</link><description>desc %d</description></item>`, i, i, i))
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}
