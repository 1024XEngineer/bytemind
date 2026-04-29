package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func newError(code ErrorCode, providerID ProviderID, message string, err error, detail string) *Error {
	return &Error{
		Code:      code,
		Provider:  providerID,
		Message:   message,
		Retryable: isRetryableCode(code),
		Err:       err,
		Detail:    strings.TrimSpace(detail),
	}
}

func isRetryableCode(code ErrorCode) bool {
	switch code {
	case ErrCodeRateLimited, ErrCodeTimeout, ErrCodeUnavailable:
		return true
	default:
		return false
	}
}

func mapError(providerID ProviderID, err error) *Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	var upstream *llm.ProviderError
	if errors.As(err, &upstream) && upstream != nil {
		return mapLLMProviderError(providerID, upstream)
	}
	var providerErr *Error
	if errors.As(err, &providerErr) && providerErr != nil {
		providerErr.Retryable = isRetryableCode(providerErr.Code)
		if providerErr.Provider == "" {
			providerErr.Provider = providerID
		}
		if providerErr.Detail == "" && providerErr.Err != nil {
			providerErr.Detail = providerErr.Err.Error()
		}
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
		return newError(ErrCodeTimeout, providerID, "provider request timed out", err, err.Error())
	}
	return newError(ErrCodeUnavailable, providerID, "provider unavailable", err, err.Error())
}

func mapLLMProviderError(providerID ProviderID, err *llm.ProviderError) *Error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(err.Message)
	switch {
	case err.Status == http.StatusUnauthorized || err.Status == http.StatusForbidden:
		return newError(ErrCodeUnauthorized, providerID, "provider unauthorized", err, detail)
	case err.Code == llm.ErrorCodeContextTooLong || err.Status == http.StatusRequestEntityTooLarge:
		return newError(ErrCodeBadRequest, providerID, "request exceeds provider context limit", err, detail)
	case err.Status == http.StatusBadRequest:
		return newError(ErrCodeBadRequest, providerID, "provider rejected request", err, detail)
	case err.Code == llm.ErrorCodeRateLimited || err.Status == http.StatusTooManyRequests:
		return newError(ErrCodeRateLimited, providerID, "provider rate limited", err, detail)
	case err.Status == http.StatusRequestTimeout || err.Status == http.StatusGatewayTimeout:
		return newError(ErrCodeTimeout, providerID, "provider request timed out", err, detail)
	case err.Status >= http.StatusInternalServerError:
		return newError(ErrCodeUnavailable, providerID, "provider unavailable", err, detail)
	default:
		return newError(ErrCodeUnavailable, providerID, "provider unavailable", err, detail)
	}
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
