package runtime

import (
	"errors"
	"fmt"

	corepkg "bytemind/internal/core"
)

const (
	// ErrorCodeNotImplemented indicates a placeholder runtime capability.
	ErrorCodeNotImplemented = "not_implemented"
	// ErrorCodeTaskNotFound indicates the task ID does not exist.
	ErrorCodeTaskNotFound = "task_not_found"
	// ErrorCodeInvalidTransition indicates an illegal status transition.
	ErrorCodeInvalidTransition = "invalid_transition"
	// ErrorCodeRetryExhausted indicates no retry budget remains.
	ErrorCodeRetryExhausted = "retry_exhausted"
	// ErrorCodeTaskTimeout is the canonical timeout code for runtime tasks.
	ErrorCodeTaskTimeout = "task_timeout"
	// ErrorCodeTaskCancelled is the canonical cancellation code for runtime tasks.
	ErrorCodeTaskCancelled = "task_cancelled"
	// ErrorCodeQuotaExceeded indicates runtime quota acquisition failure.
	ErrorCodeQuotaExceeded = "quota_exceeded"
	// ErrorCodeTaskExecutionFailed indicates task executor returned a non-timeout failure.
	ErrorCodeTaskExecutionFailed = "task_execution_failed"
)

type runtimeError struct {
	code      string
	message   string
	retryable bool
	cause     error
}

func (e *runtimeError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func (e *runtimeError) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

func (e *runtimeError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.retryable
}

func (e *runtimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newRuntimeError(code, message string, retryable bool, cause error) error {
	return &runtimeError{
		code:      code,
		message:   message,
		retryable: retryable,
		cause:     cause,
	}
}

func errorCode(err error) string {
	var semantic corepkg.SemanticError
	if errors.As(err, &semantic) {
		return semantic.Code()
	}
	return ""
}

func hasErrorCode(err error, code string) bool {
	return errorCode(err) == code
}

func taskNotFoundError(id corepkg.TaskID) error {
	return newRuntimeError(ErrorCodeTaskNotFound, fmt.Sprintf("task %q not found", id), false, nil)
}

func invalidTransitionError(from, to corepkg.TaskStatus) error {
	return newRuntimeError(
		ErrorCodeInvalidTransition,
		fmt.Sprintf("invalid task transition: %s -> %s", from, to),
		false,
		nil,
	)
}
