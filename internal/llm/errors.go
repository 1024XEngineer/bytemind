package llm

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	ErrorCodeImageTooLarge        ErrorCode = "image_too_large"
	ErrorCodeUnsupportedImage     ErrorCode = "unsupported_image"
	ErrorCodeAssetNotFound        ErrorCode = "asset_not_found"
	ErrorCodeClipboardUnavailable ErrorCode = "clipboard_unavailable"
	ErrorCodeImageDecodeFailed    ErrorCode = "image_decode_failed"
	ErrorCodeRateLimited          ErrorCode = "rate_limited"
	ErrorCodeInvalidToolCall      ErrorCode = "invalid_tool_call"
	ErrorCodeUnknown              ErrorCode = "unknown"
)

type ProviderError struct {
	Code      ErrorCode `json:"code"`
	Provider  string    `json:"provider,omitempty"`
	Message   string    `json:"message,omitempty"`
	Status    int       `json:"status,omitempty"`
	Retryable bool      `json:"retryable,omitempty"`
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Code != "" {
		return string(e.Code)
	}
	return string(ErrorCodeUnknown)
}

func WrapError(provider string, code ErrorCode, err error) *ProviderError {
	if err == nil {
		return nil
	}
	if code == "" {
		code = ErrorCodeUnknown
	}
	return &ProviderError{
		Code:     code,
		Provider: strings.TrimSpace(provider),
		Message:  err.Error(),
	}
}

func MapProviderError(provider string, status int, body string, fallback error) *ProviderError {
	code := ErrorCodeUnknown
	retryable := false
	message := strings.TrimSpace(body)
	if message == "" && fallback != nil {
		message = fallback.Error()
	}

	if status == 429 || strings.Contains(strings.ToLower(message), "rate") {
		code = ErrorCodeRateLimited
		retryable = true
	}

	if message == "" {
		message = fmt.Sprintf("provider request failed with status %d", status)
	}

	return &ProviderError{
		Code:      code,
		Provider:  strings.TrimSpace(provider),
		Message:   message,
		Status:    status,
		Retryable: retryable,
	}
}

var errInvalidRole = errors.New("invalid role")

func IsInvalidRole(err error) bool {
	return errors.Is(err, errInvalidRole)
}
