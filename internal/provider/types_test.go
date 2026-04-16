package provider

import (
	"errors"
	"testing"
)

func TestProviderErrorStringAndUnwrap(t *testing.T) {
	base := errors.New("base error")

	if (*Error)(nil).Error() != "" {
		t.Fatal("expected nil error string to be empty")
	}
	if (*Error)(nil).Unwrap() != nil {
		t.Fatal("expected nil unwrap to be nil")
	}

	withMessage := &Error{Code: ErrCodeUnavailable, Message: "trimmed message", Err: base}
	if withMessage.Error() != "trimmed message" {
		t.Fatalf("unexpected message: %q", withMessage.Error())
	}

	withWrapped := &Error{Code: ErrCodeUnavailable, Err: base}
	if withWrapped.Error() != "base error" {
		t.Fatalf("unexpected wrapped message: %q", withWrapped.Error())
	}
	if !errors.Is(withWrapped, base) {
		t.Fatal("expected unwrap to expose base error")
	}

	withCodeOnly := &Error{Code: ErrCodeBadRequest}
	if withCodeOnly.Error() != string(ErrCodeBadRequest) {
		t.Fatalf("unexpected code-only message: %q", withCodeOnly.Error())
	}
}
