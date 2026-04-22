package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	extensionspkg "bytemind/internal/extensions"
	toolspkg "bytemind/internal/tools"
)

type emptyErr struct{}

func (emptyErr) Error() string { return "" }

func TestToExtensionError(t *testing.T) {
	if toExtensionError(nil, extensionspkg.ErrCodeLoadFailed, "fallback") != nil {
		t.Fatal("expected nil error when input is nil")
	}

	original := &extensionspkg.ExtensionError{
		Code:    extensionspkg.ErrCodeLoadFailed,
		Message: "existing",
	}
	if got := toExtensionError(original, extensionspkg.ErrCodeInvalidSource, "fallback"); got != original {
		t.Fatal("expected extension error to be returned as-is")
	}

	wrapped := toExtensionError(emptyErr{}, extensionspkg.ErrCodeInvalidSource, "  fallback message  ")
	var extErr *extensionspkg.ExtensionError
	if !errors.As(wrapped, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", wrapped)
	}
	if extErr.Code != extensionspkg.ErrCodeInvalidSource {
		t.Fatalf("unexpected code: %q", extErr.Code)
	}
	if extErr.Message != "fallback message" {
		t.Fatalf("unexpected fallback message: %q", extErr.Message)
	}
}

func TestMapClientErrorToExtensionCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code extensionspkg.ErrorCode
	}{
		{
			name: "non client error",
			err:  errors.New("x"),
			code: extensionspkg.ErrCodeLoadFailed,
		},
		{
			name: "invalid config",
			err:  &ClientError{Code: ClientErrorInvalidConfig},
			code: extensionspkg.ErrCodeInvalidManifest,
		},
		{
			name: "invalid args",
			err:  &ClientError{Code: ClientErrorInvalidArgs},
			code: extensionspkg.ErrCodeInvalidManifest,
		},
		{
			name: "permission",
			err:  &ClientError{Code: ClientErrorPermission},
			code: extensionspkg.ErrCodeConflict,
		},
		{
			name: "timeout",
			err:  &ClientError{Code: ClientErrorTimeout},
			code: extensionspkg.ErrCodeBusy,
		},
		{
			name: "default",
			err:  &ClientError{Code: ClientErrorProtocol},
			code: extensionspkg.ErrCodeLoadFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapClientErrorToExtensionCode(tc.err); got != tc.code {
				t.Fatalf("expected %q, got %q", tc.code, got)
			}
		})
	}
}

func TestMapClientErrorToToolExecErrorTable(t *testing.T) {
	if mapClientErrorToToolExecError(nil) != nil {
		t.Fatal("expected nil output for nil input")
	}

	cases := []struct {
		name string
		err  error
		code toolspkg.ToolErrorCode
	}{
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			code: toolspkg.ToolErrorTimeout,
		},
		{
			name: "non client error",
			err:  errors.New("boom"),
			code: toolspkg.ToolErrorToolFailed,
		},
		{
			name: "client timeout",
			err:  &ClientError{Code: ClientErrorTimeout, Message: "timeout"},
			code: toolspkg.ToolErrorTimeout,
		},
		{
			name: "client permission",
			err:  &ClientError{Code: ClientErrorPermission, Message: "denied"},
			code: toolspkg.ToolErrorPermissionDenied,
		},
		{
			name: "client invalid args",
			err:  &ClientError{Code: ClientErrorInvalidArgs, Message: "bad"},
			code: toolspkg.ToolErrorInvalidArgs,
		},
		{
			name: "client default",
			err:  &ClientError{Code: ClientErrorProtocol, Message: "bad"},
			code: toolspkg.ToolErrorToolFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := mapClientErrorToToolExecError(tc.err)
			execErr, ok := toolspkg.AsToolExecError(mapped)
			if !ok {
				t.Fatalf("expected ToolExecError, got %T", mapped)
			}
			if execErr.Code != tc.code {
				t.Fatalf("expected %q, got %q", tc.code, execErr.Code)
			}
		})
	}
}

func TestContextError(t *testing.T) {
	if contextError(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
	if got := contextError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("expected canceled, got %v", got)
	}
	if got := contextError(context.DeadlineExceeded); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", got)
	}
	if got := contextError(errors.New("x")); got != nil {
		t.Fatalf("expected nil for unknown error, got %v", got)
	}
}

func TestSchemaAndFormattingHelpers(t *testing.T) {
	defaultSchema := normalizedSchema(nil)
	if defaultSchema["type"] != "object" {
		t.Fatalf("expected object schema, got %#v", defaultSchema)
	}
	if defaultSchema["additionalProperties"] != true {
		t.Fatalf("expected additionalProperties=true, got %#v", defaultSchema)
	}

	schema := normalizedSchema(map[string]any{
		"properties": map[string]any{"repo": map[string]any{"type": "string"}},
	})
	if schema["type"] != "object" {
		t.Fatalf("expected injected type=object, got %#v", schema)
	}

	schema = normalizedSchema(map[string]any{
		"type": "array",
	})
	if schema["type"] != "array" {
		t.Fatalf("expected existing type to be preserved, got %#v", schema)
	}

	if marshalJSONInline(map[string]any{"a": 1}) == "" {
		t.Fatal("expected marshalJSONInline to serialize map")
	}
	if marshalJSONInline(chan int(nil)) != "" {
		t.Fatal("expected marshalJSONInline to return empty string on marshal error")
	}

	if got := formatHealthMessage("  ok  ", nil); got != "ok" {
		t.Fatalf("unexpected format with nil payload: %q", got)
	}
	if got := formatHealthMessage("", map[string]any{"a": 1}); got == "" || got == " " {
		t.Fatalf("expected formatted payload, got %q", got)
	}
	if got := formatHealthMessage("status", map[string]any{"a": 1}); !strings.HasPrefix(got, "status") {
		t.Fatalf("expected prefixed message, got %q", got)
	}
	if got := formatHealthMessage("status", chan int(nil)); got != "status" {
		t.Fatalf("expected prefix fallback when payload is not marshalable, got %q", got)
	}
}
