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

func TestWebFetchToolReturnsTextContentFromHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <title>ByteMind News</title>
    <style>.hidden { display: none; }</style>
  </head>
  <body>
    <script>console.log("ignore me")</script>
    <h1>Latest Update</h1>
    <p>ByteMind adds lightweight web tools.</p>
  </body>
</html>`))
	}))
	defer server.Close()

	tool := WebFetchTool{
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"url":"`+server.URL+`","max_chars":120}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK      bool   `json:"ok"`
		Format  string `json:"format"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	decodeJSONResult(t, result, &payload)

	if !payload.OK || payload.Format != "text" {
		t.Fatalf("unexpected fetch payload %+v", payload)
	}
	if payload.Title != "ByteMind News" {
		t.Fatalf("unexpected title %q", payload.Title)
	}
	if strings.Contains(payload.Content, "console.log") || strings.Contains(payload.Content, "<h1>") {
		t.Fatalf("expected cleaned text content, got %q", payload.Content)
	}
	if !strings.Contains(payload.Content, "ByteMind adds lightweight web tools.") {
		t.Fatalf("unexpected content %q", payload.Content)
	}
}

func TestWebFetchToolReturnsHTTPErrorPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	tool := WebFetchTool{
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"url":"`+server.URL+`"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
		Status    int    `json:"status_code"`
	}
	decodeJSONResult(t, result, &payload)

	if payload.OK || payload.ErrorCode != "http_not_found" || payload.Status != http.StatusNotFound {
		t.Fatalf("unexpected http failure payload %+v", payload)
	}
}

func TestWebFetchToolReturnsUnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x01, 0x02, 0x03})
	}))
	defer server.Close()

	tool := WebFetchTool{
		HTTPClient: server.Client(),
	}

	result, err := tool.Run(context.Background(), []byte(`{"url":"`+server.URL+`"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
	}
	decodeJSONResult(t, result, &payload)

	if payload.OK || payload.ErrorCode != "unsupported_content_type" {
		t.Fatalf("unexpected unsupported type payload %+v", payload)
	}
}

func TestWebFetchToolReturnsNetworkUnreachablePayload(t *testing.T) {
	tool := WebFetchTool{
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

	result, err := tool.Run(context.Background(), []byte(`{"url":"https://example.com/news"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
	}
	decodeJSONResult(t, result, &payload)

	if payload.OK || payload.ErrorCode != "network_unreachable" {
		t.Fatalf("unexpected network failure payload %+v", payload)
	}
}
