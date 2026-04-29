package provider

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestMapErrorNormalizesRetryableByCode(t *testing.T) {
	mapped := mapError("openai", &Error{Code: ErrCodeBadRequest, Provider: "openai", Message: "bad", Retryable: true, Err: errors.New("raw")})
	if mapped.Code != ErrCodeBadRequest || mapped.Retryable {
		t.Fatalf("unexpected mapped error %#v", mapped)
	}
	mapped = mapError("openai", &Error{Code: ErrCodeUnavailable, Message: "wrapped", Retryable: true, Err: &llm.ProviderError{Status: 401, Message: "bad auth"}})
	if mapped.Code != ErrCodeUnauthorized || mapped.Retryable || mapped.Provider != "openai" || mapped.Detail != "bad auth" {
		t.Fatalf("expected wrapped provider error to normalize from upstream status, got %#v", mapped)
	}
}

func TestMapErrorHandlesTimeoutSources(t *testing.T) {
	mapped := mapError("openai", context.DeadlineExceeded)
	if mapped.Code != ErrCodeTimeout || !mapped.Retryable {
		t.Fatalf("unexpected deadline mapping %#v", mapped)
	}
	mapped = mapError("openai", timeoutError{})
	if mapped.Code != ErrCodeTimeout || !mapped.Retryable {
		t.Fatalf("unexpected net timeout mapping %#v", mapped)
	}
	if mapped := mapError("openai", context.Canceled); mapped != nil {
		t.Fatalf("expected context cancellation to bypass provider mapping, got %#v", mapped)
	}
}

func TestMapLLMProviderErrorStatusRules(t *testing.T) {
	tests := []struct {
		name      string
		err       *llm.ProviderError
		code      ErrorCode
		retryable bool
		message   string
	}{
		{name: "unauthorized", err: &llm.ProviderError{Status: 401, Message: "bad auth"}, code: ErrCodeUnauthorized, retryable: false, message: "provider unauthorized"},
		{name: "forbidden", err: &llm.ProviderError{Status: 403, Message: "forbidden"}, code: ErrCodeUnauthorized, retryable: false, message: "provider unauthorized"},
		{name: "bad request", err: &llm.ProviderError{Status: 400, Message: "invalid"}, code: ErrCodeBadRequest, retryable: false, message: "provider rejected request"},
		{name: "too long", err: &llm.ProviderError{Code: llm.ErrorCodeContextTooLong, Status: 413, Message: "too many tokens"}, code: ErrCodeBadRequest, retryable: false, message: "request exceeds provider context limit"},
		{name: "rate limit", err: &llm.ProviderError{Status: 429, Message: "slow down"}, code: ErrCodeRateLimited, retryable: true, message: "provider rate limited"},
		{name: "timeout", err: &llm.ProviderError{Status: 504, Message: "gateway timeout"}, code: ErrCodeTimeout, retryable: true, message: "provider request timed out"},
		{name: "server error", err: &llm.ProviderError{Status: 500, Message: "boom"}, code: ErrCodeUnavailable, retryable: true, message: "provider unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := mapLLMProviderError("anthropic", tt.err)
			if mapped.Code != tt.code || mapped.Retryable != tt.retryable || mapped.Message != tt.message || mapped.Detail != tt.err.Message {
				t.Fatalf("unexpected mapped error %#v", mapped)
			}
		})
	}
}

func TestTimeoutErrorSatisfiesNetError(t *testing.T) {
	var netErr net.Error = timeoutError{}
	if !netErr.Timeout() {
		t.Fatal("expected timeout net error")
	}
}
