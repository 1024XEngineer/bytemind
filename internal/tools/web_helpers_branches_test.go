package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"testing"
	"time"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestWebHelpersBranches(t *testing.T) {
	defaultClient := webClientWithTimeout(nil, 0)
	if defaultClient.Timeout != defaultWebToolTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultWebToolTimeout, defaultClient.Timeout)
	}

	base := &http.Client{Timeout: 2 * time.Second}
	cloned := webClientWithTimeout(base, 5*time.Second)
	if cloned == base {
		t.Fatal("expected copied client, got same pointer")
	}
	if cloned.Timeout != 5*time.Second {
		t.Fatalf("expected overridden timeout, got %s", cloned.Timeout)
	}
	if base.Timeout != 2*time.Second {
		t.Fatalf("expected base timeout unchanged, got %s", base.Timeout)
	}

	rawFailure, err := webFailureResult(" ", " ", map[string]any{"query": "go"})
	if err != nil {
		t.Fatal(err)
	}
	var failurePayload map[string]any
	if err := json.Unmarshal([]byte(rawFailure), &failurePayload); err != nil {
		t.Fatalf("decode failure payload: %v", err)
	}
	if failurePayload["error_code"] != "unknown_error" || failurePayload["error"] != "web tool failed" || failurePayload["query"] != "go" {
		t.Fatalf("unexpected failure payload %+v", failurePayload)
	}

	if code := webNetworkErrorCode(context.DeadlineExceeded); code != "request_timeout" {
		t.Fatalf("expected request_timeout, got %q", code)
	}
	timeoutWrapped := &neturl.Error{
		Op:  "Get",
		URL: "https://example.com",
		Err: fmt.Errorf("wrapped: %w", timeoutNetError{}),
	}
	if code := webNetworkErrorCode(timeoutWrapped); code != "request_timeout" {
		t.Fatalf("expected request_timeout for wrapped timeout, got %q", code)
	}
	if code := webNetworkErrorCode(errors.New("network unreachable")); code != "network_unreachable" {
		t.Fatalf("expected network_unreachable, got %q", code)
	}
	if isWebTimeoutError(nil) {
		t.Fatal("expected nil error not to be timeout")
	}

	statusCases := map[int]string{
		http.StatusNotFound:            "http_not_found",
		http.StatusUnauthorized:        "http_unauthorized",
		http.StatusForbidden:           "http_forbidden",
		http.StatusTooManyRequests:     "upstream_rate_limited",
		http.StatusInternalServerError: "upstream_error",
		http.StatusBadRequest:          "http_error",
		http.StatusOK:                  "unknown_error",
	}
	for status, want := range statusCases {
		if got := webHTTPStatusErrorCode(status); got != want {
			t.Fatalf("status=%d expected=%q got=%q", status, want, got)
		}
	}

	if got := collapseWhitespace(" \n\t "); got != "" {
		t.Fatalf("expected empty whitespace collapse, got %q", got)
	}
	if got := collapseWhitespace(" a \n  b\t c "); got != "a b c" {
		t.Fatalf("unexpected collapsed result %q", got)
	}

	if got, truncated := truncateRunes("   ", 0); got != "" || truncated {
		t.Fatalf("expected blank non-truncated result, got=%q truncated=%v", got, truncated)
	}
	if got, truncated := truncateRunes("hello", 0); got != "" || !truncated {
		t.Fatalf("expected truncated result when limit<=0, got=%q truncated=%v", got, truncated)
	}
	if got, truncated := truncateRunes("猫咪编程", 2); got != "猫咪" || !truncated {
		t.Fatalf("expected rune truncation result, got=%q truncated=%v", got, truncated)
	}

	var parsed struct {
		Name string `json:"name"`
	}
	failureJSON, err := parseJSONOrFailure([]byte(`{"name":"ok"}`), &parsed, map[string]any{"source": "unit"})
	if err != nil {
		t.Fatal(err)
	}
	if failureJSON != "" || parsed.Name != "ok" {
		t.Fatalf("expected successful parse, failure=%q parsed=%+v", failureJSON, parsed)
	}

	failureJSON, err = parseJSONOrFailure([]byte(`{"name":`), &parsed, map[string]any{"source": "unit"})
	if err != nil {
		t.Fatal(err)
	}
	var invalidPayload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"error_code"`
		Source    string `json:"source"`
	}
	decodeJSONResult(t, failureJSON, &invalidPayload)
	if invalidPayload.OK || invalidPayload.ErrorCode != "provider_invalid_response" || invalidPayload.Source != "unit" {
		t.Fatalf("unexpected invalid JSON payload %+v", invalidPayload)
	}
}
