package storage

import (
	"context"
	"errors"
	"fmt"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type ErrorCode string

const (
	ErrCodeLockTimeout ErrorCode = "lock_timeout"
)

type UnlockFunc func() error

type Locker interface {
	// LockSession acquires the session-scoped lock.
	// acquireCtx is only for acquisition waiting/cancellation.
	// Implementations must not bind lock lifetime to acquireCtx after acquisition.
	LockSession(acquireCtx context.Context, sessionID corepkg.SessionID) (UnlockFunc, error)
	// LockTask acquires the task-scoped lock.
	// acquireCtx is only for acquisition waiting/cancellation.
	// Implementations must not bind lock lifetime to acquireCtx after acquisition.
	LockTask(acquireCtx context.Context, taskID corepkg.TaskID) (UnlockFunc, error)
}

type lockerError struct {
	code      ErrorCode
	message   string
	retryable bool
	cause     error
}

func (e *lockerError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func (e *lockerError) Code() string {
	if e == nil {
		return ""
	}
	return string(e.code)
}

func (e *lockerError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.retryable
}

func (e *lockerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newLockerError(code ErrorCode, message string, retryable bool, cause error) error {
	return &lockerError{
		code:      code,
		message:   message,
		retryable: retryable,
		cause:     cause,
	}
}

func hasErrorCode(err error, code ErrorCode) bool {
	var semantic corepkg.SemanticError
	if !errors.As(err, &semantic) {
		return false
	}
	return semantic.Code() == string(code)
}
