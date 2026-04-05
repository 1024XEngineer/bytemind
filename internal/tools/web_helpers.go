package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	defaultWebToolTimeout = 15 * time.Second
	defaultWebToolUA      = "ByteMind/1.0"
)

func webClientWithTimeout(base *http.Client, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultWebToolTimeout
	}
	if base == nil {
		return &http.Client{Timeout: timeout}
	}
	client := *base
	client.Timeout = timeout
	return &client
}

func webFailureResult(code, message string, extra map[string]any) (string, error) {
	payload := map[string]any{
		"ok":         false,
		"error_code": strings.TrimSpace(code),
		"error":      strings.TrimSpace(message),
	}
	if payload["error_code"] == "" {
		payload["error_code"] = "unknown_error"
	}
	if payload["error"] == "" {
		payload["error"] = "web tool failed"
	}
	for k, v := range extra {
		payload[k] = v
	}
	return toJSON(payload)
}

func webNetworkErrorCode(err error) string {
	if isWebTimeoutError(err) {
		return "request_timeout"
	}
	return "network_unreachable"
}

func isWebTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return true
		}
		var wrappedNetErr net.Error
		if errors.As(urlErr.Err, &wrappedNetErr) && wrappedNetErr.Timeout() {
			return true
		}
	}
	return false
}

func webHTTPStatusErrorCode(status int) string {
	switch status {
	case http.StatusNotFound:
		return "http_not_found"
	case http.StatusUnauthorized:
		return "http_unauthorized"
	case http.StatusForbidden:
		return "http_forbidden"
	case http.StatusTooManyRequests:
		return "upstream_rate_limited"
	}
	if status >= 500 {
		return "upstream_error"
	}
	if status >= 400 {
		return "http_error"
	}
	return "unknown_error"
}

func collapseWhitespace(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	var builder strings.Builder
	lastSpace := true
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !lastSpace {
				builder.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		builder.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(builder.String())
}

func truncateRunes(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimSpace(text) != ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

func parseJSONOrFailure(raw []byte, target any, context map[string]any) (string, error) {
	if err := json.Unmarshal(raw, target); err != nil {
		return webFailureResult("provider_invalid_response", "web provider returned invalid JSON", context)
	}
	return "", nil
}
