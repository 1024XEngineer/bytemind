package tokenusage

import "fmt"

type ErrorCode string

const (
	ErrCodeInvalidConfig ErrorCode = "invalid_config"
	ErrCodeInvalidInput  ErrorCode = "invalid_input"
	ErrCodeStorage       ErrorCode = "storage_error"
	ErrCodeTimeout       ErrorCode = "timeout"
	ErrCodeNotFound      ErrorCode = "not_found"
	ErrCodeInternal      ErrorCode = "internal_error"
)

type UsageError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *UsageError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

func (e *UsageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapError(code ErrorCode, message string, err error) error {
	return &UsageError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}
