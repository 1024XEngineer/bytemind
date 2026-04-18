package extensions

import "fmt"

type ErrorCode string

const (
	ErrCodeInvalidSource     ErrorCode = "invalid_source"
	ErrCodeInvalidManifest   ErrorCode = "invalid_manifest"
	ErrCodeInvalidExtension  ErrorCode = "invalid_extension"
	ErrCodeInvalidTransition ErrorCode = "invalid_transition"
	ErrCodeNotFound          ErrorCode = "not_found"
	ErrCodeDuplicate         ErrorCode = "duplicate_extension"
	ErrCodeAlreadyLoaded     ErrorCode = "already_loaded"
	ErrCodeConflict          ErrorCode = "conflict"
	ErrCodeBusy              ErrorCode = "busy"
	ErrCodeLoadFailed        ErrorCode = "load_failed"
	ErrCodeUnloadFailed      ErrorCode = "unload_failed"
)

type ExtensionError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *ExtensionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

func (e *ExtensionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ExtensionError) CodeString() string {
	if e == nil {
		return ""
	}
	return string(e.Code)
}

func wrapError(code ErrorCode, message string, err error) error {
	return &ExtensionError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}
