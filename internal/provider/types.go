package provider

import (
	"errors"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
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

type ModelMetadata struct {
	ProviderID      ProviderID
	ModelID         ModelID
	Family          string
	ContextWindow   int
	MaxOutputTokens int
	SupportsTools   bool
	UsageSource     string
	Metadata        map[string]string
}

type ModelInfo struct {
	ProviderID   ProviderID
	ModelID      ModelID
	DisplayAlias string
	Metadata     map[string]string
}

func (m ModelInfo) ModelMetadata() ModelMetadata {
	metadata := cloneMetadataMap(m.Metadata)
	return ModelMetadata{
		ProviderID:      m.ProviderID,
		ModelID:         m.ModelID,
		Family:          firstNonEmptyMetadata(metadata["family"], metadata["provider_family"]),
		ContextWindow:   parseMetadataInt(metadata, "context_window"),
		MaxOutputTokens: parseMetadataInt(metadata, "max_output_tokens"),
		SupportsTools:   parseMetadataBool(metadata, "supports_tools"),
		UsageSource:     firstNonEmptyMetadata(metadata["usage_source"], metadata["source"]),
		Metadata:        metadata,
	}
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

func cloneMetadataMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmptyMetadata(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseMetadataInt(metadata map[string]string, key string) int {
	raw := strings.TrimSpace(metadata[key])
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if value < 0 {
		return 0
	}
	return value
}

func parseMetadataBool(metadata map[string]string, key string) bool {
	raw := strings.TrimSpace(metadata[key])
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}
