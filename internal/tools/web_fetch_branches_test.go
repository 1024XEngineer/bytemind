package tools

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingReadCloser struct {
	err error
}

func (f failingReadCloser) Read([]byte) (int, error) {
	return 0, f.err
}

func (f failingReadCloser) Close() error {
	return nil
}

func TestWebFetchHelpersAndValidation(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr string
		wantURL string
	}{
		{raw: " ", wantErr: "url is required"},
		{raw: "example.com/docs", wantURL: "https://example.com/docs"},
		{raw: "/docs", wantErr: "url must be absolute"},
		{raw: "ftp://example.com/file", wantErr: "url scheme must be http or https"},
		{raw: "http://example.com/x", wantURL: "http://example.com/x"},
	}
	for _, tc := range tests {
		got, err := normalizeWebURL(tc.raw)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("raw=%q expected err containing %q, got %v", tc.raw, tc.wantErr, err)
			}
			continue
		}
		if err != nil || got != tc.wantURL {
			t.Fatalf("raw=%q expected url=%q err=nil, got url=%q err=%v", tc.raw, tc.wantURL, got, err)
		}
	}

	if got := normalizeContentType(" text/html; charset=utf-8 "); got != "text/html" {
		t.Fatalf("unexpected content type normalization %q", got)
	}
	if !isTextualContentType("application/json") || isTextualContentType("application/octet-stream") {
		t.Fatal("unexpected textual content-type detection")
	}
	if !looksLikeHTML("", "<!doctype html><html></html>") || !looksLikeHTML("", "<html><body>x</body></html>") {
		t.Fatal("expected html body detection")
	}
	if looksLikeHTML("", "plain text") {
		t.Fatal("expected plain text not to be html")
	}
	if title := extractHTMLTitle("<html><head></head></html>"); title != "" {
		t.Fatalf("expected empty title, got %q", title)
	}
	if title := extractHTMLTitle("<title> Go &amp; Tools </title>"); title != "Go & Tools" {
		t.Fatalf("unexpected title %q", title)
	}
	if text := htmlToText(`<style>.x{}</style><script>x()</script><h1>Go</h1><p>tools</p>`); text != "Go tools" {
		t.Fatalf("unexpected htmlToText output %q", text)
	}
}

func TestWebFetchRunBranches(t *testing.T) {
	tool := WebFetchTool{}
	if _, err := tool.Run(context.Background(), []byte(`{"url":"https://example.com","format":"markdown"}`), &ExecutionContext{}); err == nil {
		t.Fatal("expected invalid format error")
	}

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("   <html><body><h1>Hello</h1></body></html>   "))
	}))
	defer htmlServer.Close()

	tool = WebFetchTool{HTTPClient: htmlServer.Client()}
	result, err := tool.Run(context.Background(), []byte(`{"url":"`+htmlServer.URL+`","format":"html","max_chars":200}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var htmlPayload struct {
		OK      bool   `json:"ok"`
		Format  string `json:"format"`
		Content string `json:"content"`
	}
	decodeJSONResult(t, result, &htmlPayload)
	if !htmlPayload.OK || htmlPayload.Format != "html" || htmlPayload.Content != "<html><body><h1>Hello</h1></body></html>" {
		t.Fatalf("unexpected html payload %+v", htmlPayload)
	}

	textServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("a    b    c"))
	}))
	defer textServer.Close()

	tool = WebFetchTool{HTTPClient: textServer.Client()}
	result, err = tool.Run(context.Background(), []byte(`{"url":"`+textServer.URL+`","max_chars":3}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var textPayload struct {
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	decodeJSONResult(t, result, &textPayload)
	if textPayload.Content != "a b" || !textPayload.Truncated {
		t.Fatalf("unexpected text payload %+v", textPayload)
	}

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer redirectServer.Close()
	tool = WebFetchTool{HTTPClient: redirectServer.Client()}
	result, err = tool.Run(context.Background(), []byte(`{"url":"`+redirectServer.URL+`/start"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var redirectPayload struct {
		RequestedURL string `json:"requested_url"`
		URL          string `json:"url"`
	}
	decodeJSONResult(t, result, &redirectPayload)
	if redirectPayload.RequestedURL != redirectServer.URL+"/start" || redirectPayload.URL != redirectServer.URL+"/final" {
		t.Fatalf("unexpected redirect payload %+v", redirectPayload)
	}

	tool = WebFetchTool{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       failingReadCloser{err: errors.New("boom")},
					Request:    req,
				}, nil
			}),
		},
	}
	result, err = tool.Run(context.Background(), []byte(`{"url":"https://example.com"}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var readErrPayload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
	}
	decodeJSONResult(t, result, &readErrPayload)
	if readErrPayload.OK || readErrPayload.ErrorCode != "upstream_error" {
		t.Fatalf("unexpected read-error payload %+v", readErrPayload)
	}
}

func TestWebFetchRunClampsBodyAndChars(t *testing.T) {
	largeBody := strings.Repeat("x", maxWebFetchBodyBytes+64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	tool := WebFetchTool{HTTPClient: server.Client()}
	result, err := tool.Run(context.Background(), []byte(`{"url":"`+server.URL+`","max_chars":999999,"timeout_seconds":999}`), &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	decodeJSONResult(t, result, &payload)
	if len(payload.Content) != maxWebFetchMaxChars {
		t.Fatalf("expected clamped max chars=%d, got %d", maxWebFetchMaxChars, len(payload.Content))
	}
	if !payload.Truncated {
		t.Fatalf("expected truncated=true for oversized body, got %+v", payload)
	}
}
