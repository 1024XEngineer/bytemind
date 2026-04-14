package llm

import (
	"errors"
	"testing"
)

func TestProviderErrorErrorFallbackOrder(t *testing.T) {
	if (&ProviderError{Message: "msg", Code: ErrorCodeRateLimited}).Error() != "msg" {
		t.Fatal("expected message to win")
	}
	if (&ProviderError{Code: ErrorCodeAssetNotFound}).Error() != string(ErrorCodeAssetNotFound) {
		t.Fatal("expected code fallback")
	}
	if (&ProviderError{}).Error() != string(ErrorCodeUnknown) {
		t.Fatal("expected unknown fallback")
	}
}

func TestWrapErrorAndMapProviderError(t *testing.T) {
	if WrapError("openai", ErrorCodeUnknown, nil) != nil {
		t.Fatal("expected nil wrap for nil error")
	}
	wrapped := WrapError(" openai ", "", errors.New("boom"))
	if wrapped.Code != ErrorCodeUnknown || wrapped.Provider != "openai" || wrapped.Message != "boom" {
		t.Fatalf("unexpected wrapped error: %#v", wrapped)
	}

	mapped := MapProviderError("anthropic", 429, " rate limited ", nil)
	if mapped.Code != ErrorCodeRateLimited || !mapped.Retryable || mapped.Status != 429 {
		t.Fatalf("unexpected mapped error: %#v", mapped)
	}

	mappedFallback := MapProviderError("x", 500, "", errors.New("upstream failed"))
	if mappedFallback.Message != "upstream failed" {
		t.Fatalf("expected fallback message, got %#v", mappedFallback)
	}
}

func TestMapProviderErrorContextTooLong(t *testing.T) {
	mapped := MapProviderError("openai", 400, "maximum context length exceeded", nil)
	if mapped.Code != ErrorCodeContextTooLong || mapped.Retryable {
		t.Fatalf("expected context too long mapping, got %#v", mapped)
	}

	mappedStatus := MapProviderError("openai", 413, "", errors.New("payload too large"))
	if mappedStatus.Code != ErrorCodeContextTooLong {
		t.Fatalf("expected status-based context too long mapping, got %#v", mappedStatus)
	}
}

func TestIsContextTooLongMessage(t *testing.T) {
	if !IsContextTooLongMessage("Prompt is too long for this context window") {
		t.Fatal("expected known context-too-long phrase to match")
	}
	if IsContextTooLongMessage("rate limit exceeded") {
		t.Fatal("expected unrelated phrase not to match")
	}
}

func TestIsInvalidRole(t *testing.T) {
	if !IsInvalidRole(errInvalidRole) {
		t.Fatal("expected direct invalid role match")
	}
	if IsInvalidRole(errors.New("x")) {
		t.Fatal("expected non-matching error")
	}
}
