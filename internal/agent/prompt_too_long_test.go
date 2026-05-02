package agent

import (
	"errors"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestIsPromptTooLongError(t *testing.T) {
	if !isPromptTooLongError(newPromptTooLongError(100, 120, 0.95)) {
		t.Fatal("expected local prompt too long error to match")
	}
	if !isPromptTooLongError(&llm.ProviderError{Code: llm.ErrorCodeContextTooLong, Message: "context exceeded"}) {
		t.Fatal("expected provider context too long code to match")
	}
	if !isPromptTooLongError(errors.New("maximum context length exceeded")) {
		t.Fatal("expected fallback text match")
	}
	if isPromptTooLongError(errors.New("timeout waiting for upstream")) {
		t.Fatal("expected non-matching error to be ignored")
	}
}
