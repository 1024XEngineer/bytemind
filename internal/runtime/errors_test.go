package runtime

import (
	"errors"
	"strings"
	"testing"

	corepkg "bytemind/internal/core"
)

func TestRuntimeErrorImplementsSemanticErrorAndWrapsCause(t *testing.T) {
	cause := errors.New("boom")
	err := newRuntimeError(ErrorCodeTaskTimeout, "task timed out", true, cause)

	var semantic corepkg.SemanticError
	if !errors.As(err, &semantic) {
		t.Fatal("expected runtime error to implement SemanticError")
	}
	if semantic.Code() != ErrorCodeTaskTimeout {
		t.Fatalf("expected code %q, got %q", ErrorCodeTaskTimeout, semantic.Code())
	}
	if !semantic.Retryable() {
		t.Fatal("expected retryable=true")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected wrapped cause to be discoverable with errors.Is")
	}
	if !strings.Contains(err.Error(), "task timed out") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected formatted error to include message and cause, got %q", err.Error())
	}
}

func TestRuntimeErrorHelpersHandleNonSemanticErrors(t *testing.T) {
	plain := errors.New("plain")

	if code := errorCode(plain); code != "" {
		t.Fatalf("expected empty code for plain error, got %q", code)
	}
	if hasErrorCode(plain, ErrorCodeTaskNotFound) {
		t.Fatal("expected hasErrorCode to be false for plain error")
	}
}
