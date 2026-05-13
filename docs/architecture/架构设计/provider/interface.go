//go:build ignore

package provider

import (
	"context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

type ProviderID string
type ModelID string

type ErrorCode string

const (
	ErrCodeUnauthorized ErrorCode = "unauthorized"
	ErrCodeRateLimited  ErrorCode = "rate_limited"
	ErrCodeTimeout      ErrorCode = "timeout"
	ErrCodeUnavailable  ErrorCode = "unavailable"
	ErrCodeBadRequest   ErrorCode = "bad_request"
)

type Message struct {
	Role       core.Role
	Parts      []core.MessagePart
	Name       string
	ToolCallID string
}

type ToolSpec struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type Request struct {
	SessionID       core.SessionID
	TraceID         core.TraceID
	ModelID         ModelID
	Messages        []Message
	Tools           []ToolSpec
	MaxOutputTokens int
	Temperature     float64
	Metadata        map[string]string
}

type EventType string

const (
	EventStart    EventType = "start"
	EventDelta    EventType = "delta"
	EventToolCall EventType = "tool_call"
	EventUsage    EventType = "usage"
	EventResult   EventType = "result"
	EventError    EventType = "error"
)

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
}

type Event struct {
	Type      EventType
	Meta      core.EventMeta
	Payload   json.RawMessage
	Usage     *Usage
	ErrorCode string
	Retryable bool
}

type ModelInfo struct {
	ProviderID      ProviderID
	ModelID         ModelID
	DisplayName     string
	MaxInputTokens  int
	MaxOutputTokens int
	SupportsTools   bool
	UpdatedAt       time.Time
}

type RouteResult struct {
	ProviderID ProviderID
	ModelID    ModelID
	Fallbacks  []ModelID
}

type Client interface {
	ProviderID() ProviderID
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Registry interface {
	Register(ctx context.Context, client Client) error
	Get(ctx context.Context, id ProviderID) (Client, bool)
	List(ctx context.Context) ([]ProviderID, error)
}

type Router interface {
	Route(ctx context.Context, requestedModel ModelID, metadata map[string]string) (RouteResult, error)
}

type HealthChecker interface {
	Check(ctx context.Context, id ProviderID) error
}
