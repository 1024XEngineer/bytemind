package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeToolErrorContractTimeout(t *testing.T) {
	cause := context.DeadlineExceeded
	err := normalizeToolError(cause)
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorTimeout {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !execErr.Retryable {
		t.Fatal("timeout should be retryable")
	}
	if !errors.Is(execErr, cause) {
		t.Fatal("timeout cause must be preserved")
	}
}

func TestNormalizeToolErrorContractPermissionDenied(t *testing.T) {
	cause := errors.New("approval denied")
	err := normalizeToolError(cause)
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if execErr.Retryable {
		t.Fatal("permission denied should not be retryable")
	}
	if !errors.Is(execErr, cause) {
		t.Fatal("permission-denied cause must be preserved")
	}
}

func TestNormalizeToolErrorContractInvalidArgs(t *testing.T) {
	cause := errors.New("unknown argument \"extra\"")
	err := normalizeToolError(cause)
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if execErr.Retryable {
		t.Fatal("invalid args should not be retryable")
	}
	if !errors.Is(execErr, cause) {
		t.Fatal("invalid-args cause must be preserved")
	}
}

func TestNormalizeToolErrorContractToolFailed(t *testing.T) {
	cause := errors.New("tool crashed")
	err := normalizeToolError(cause)
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorToolFailed {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !execErr.Retryable {
		t.Fatal("tool failed should be retryable")
	}
	if !errors.Is(execErr, cause) {
		t.Fatal("tool-failed cause must be preserved")
	}
}

func TestNormalizeToolErrorKeepsExistingToolExecError(t *testing.T) {
	original := NewToolExecError(ToolErrorInvalidArgs, "bad input", false, errors.New("root"))
	err := normalizeToolError(original)
	if err != original {
		t.Fatal("expected existing ToolExecError to be returned unchanged")
	}
}
