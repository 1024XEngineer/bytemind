package provider

import (
	"errors"
	"strings"

	"bytemind/internal/llm"
)

const (
	ProviderOpenAI    ProviderID = "openai"
	ProviderAnthropic ProviderID = "anthropic"
	ProviderGemini    ProviderID = "gemini"
)

type ErrorCode string

type EventType string

type HealthStatus string

const (
	ErrCodeUnauthorized      ErrorCode = "unauthorized"
	ErrCodeRateLimited       ErrorCode = "rate_limited"
	ErrCodeTimeout           ErrorCode = "timeout"
	ErrCodeUnavailable       ErrorCode = "unavailable"
	ErrCodeBadRequest        ErrorCode = "bad_request"
	ErrCodeProviderNotFound  ErrorCode = "provider_not_found"
	ErrCodeDuplicateProvider ErrorCode = "duplicate_provider"
)

const (
	EventStart    EventType = "start"
	EventDelta    EventType = "delta"
	EventToolCall EventType = "tool_call"
	EventUsage    EventType = "usage"
	EventResult   EventType = "result"
	EventError    EventType = "error"
)

const (
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusDegraded    HealthStatus = "degraded"
	HealthStatusUnavailable HealthStatus = "unavailable"
	HealthStatusHalfOpen    HealthStatus = "half_open"
)

var (
	ErrProviderNotFound  = errors.New(string(ErrCodeProviderNotFound))
	ErrDuplicateProvider = errors.New(string(ErrCodeDuplicateProvider))
)

type ModelInfo struct {
	ProviderID   ProviderID
	ModelID      ModelID
	DisplayAlias string
	Metadata     map[string]string
}

type Warning struct {
	ProviderID ProviderID
	Reason     string
}

type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Cost         float64
	Currency     string
	IsEstimated  bool
}

type Error struct {
	Code      ErrorCode
	Provider  ProviderID
	Message   string
	Retryable bool
	Err       error
	Detail    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Event struct {
	ID         string
	TraceID    string
	ProviderID ProviderID
	ModelID    ModelID
	Type       EventType
	Delta      string
	ToolCall   *llm.ToolCall
	Usage      *Usage
	Result     *llm.Message
	Error      *Error
}
