package tools

import (
	"errors"
	"fmt"
	"strings"
)

type ToolErrorCode string

const (
	ToolErrorInvalidArgs      ToolErrorCode = "invalid_args"
	ToolErrorPermissionDenied ToolErrorCode = "permission_denied"
	ToolErrorTimeout          ToolErrorCode = "timeout"
	ToolErrorToolFailed       ToolErrorCode = "tool_failed"
	ToolErrorInternal         ToolErrorCode = "internal_error"
)

type ToolExecError struct {
	Code      ToolErrorCode
	Message   string
	Retryable bool
	Cause     error
}

func (e *ToolExecError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(string(e.Code))
	msg := strings.TrimSpace(e.Message)
	switch {
	case code == "":
		return msg
	case msg == "":
		return code
	default:
		return fmt.Sprintf("%s: %s", code, msg)
	}
}

func (e *ToolExecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewToolExecError(code ToolErrorCode, message string, retryable bool, cause error) *ToolExecError {
	return &ToolExecError{
		Code:      code,
		Message:   strings.TrimSpace(message),
		Retryable: retryable,
		Cause:     cause,
	}
}

func AsToolExecError(err error) (*ToolExecError, bool) {
	var execErr *ToolExecError
	if !errors.As(err, &execErr) || execErr == nil {
		return nil, false
	}
	return execErr, true
}
