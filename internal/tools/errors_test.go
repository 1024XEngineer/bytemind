package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestToolExecErrorFormatsCodeAndMessage(t *testing.T) {
	err := NewToolExecError(ToolErrorInvalidArgs, "  bad input  ", false, nil)
	if err.Error() != "invalid_args: bad input" {
		t.Fatalf("unexpected formatted error: %q", err.Error())
	}
	if err.Retryable {
		t.Fatal("expected retryable=false")
	}
}

func TestToolExecErrorUnwrapAndAs(t *testing.T) {
	cause := errors.New("root cause")
	base := NewToolExecError(ToolErrorToolFailed, "tool crashed", true, cause)
	wrapped := fmt.Errorf("wrapped: %w", base)

	execErr, ok := AsToolExecError(wrapped)
	if !ok {
		t.Fatalf("expected wrapped ToolExecError, got %T", wrapped)
	}
	if execErr.Code != ToolErrorToolFailed {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !errors.Is(execErr, cause) {
		t.Fatalf("expected errors.Is to match cause")
	}
	if !execErr.Retryable {
		t.Fatal("expected retryable=true")
	}
}

func TestNormalizeToolErrorMappings(t *testing.T) {
	cases := []struct {
		name      string
		input     error
		wantCode  ToolErrorCode
		retryable bool
	}{
		{
			name:      "timeout",
			input:     context.DeadlineExceeded,
			wantCode:  ToolErrorTimeout,
			retryable: true,
		},
		{
			name:      "permission",
			input:     errors.New("permission denied by policy"),
			wantCode:  ToolErrorPermissionDenied,
			retryable: false,
		},
		{
			name:      "invalid args",
			input:     errors.New(`unknown argument "x"`),
			wantCode:  ToolErrorInvalidArgs,
			retryable: false,
		},
		{
			name:      "generic failure",
			input:     errors.New("unexpected execution failure"),
			wantCode:  ToolErrorToolFailed,
			retryable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeToolError(tc.input)
			execErr, ok := AsToolExecError(err)
			if !ok {
				t.Fatalf("expected ToolExecError, got %T", err)
			}
			if execErr.Code != tc.wantCode {
				t.Fatalf("unexpected code: got=%s want=%s", execErr.Code, tc.wantCode)
			}
			if execErr.Retryable != tc.retryable {
				t.Fatalf("unexpected retryable: got=%v want=%v", execErr.Retryable, tc.retryable)
			}
			if !errors.Is(execErr, tc.input) {
				t.Fatalf("expected normalized error to unwrap original input")
			}
		})
	}
}

func TestNormalizeToolErrorPassThroughExistingToolExecError(t *testing.T) {
	original := NewToolExecError(ToolErrorPermissionDenied, "already normalized", false, nil)
	got := normalizeToolError(original)
	if got != original {
		t.Fatal("expected normalizeToolError to return existing ToolExecError unchanged")
	}
}
