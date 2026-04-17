package provider

import (
	"context"
	"time"

	"bytemind/internal/llm"
)

type ProviderID string

type ModelID string

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
	Route(ctx context.Context, requestedModel ModelID, rc RouteContext) (RouteResult, error)
}

type HealthChecker interface {
	Check(ctx context.Context, id ProviderID) error
	Status(ctx context.Context, id ProviderID) HealthSnapshot
	RecordSuccess(ctx context.Context, id ProviderID)
	RecordFailure(ctx context.Context, id ProviderID, err error)
}

type Request struct {
	llm.ChatRequest
	TraceID string
	Tags    map[string]string
}

type RouteContext struct {
	Scenario      string
	Region        string
	PreferLatency bool
	PreferLowCost bool
	AllowFallback bool
	Tags          map[string]string
}

type RouteResult struct {
	Primary   RouteTarget
	Fallbacks []RouteTarget
}

type RouteTarget struct {
	ProviderID ProviderID
	ModelID    ModelID
	Client     Client
}

type HealthConfig struct {
	CheckIntervalSec        int
	FailThreshold           int
	RecoverProbeSec         int
	RecoverSuccessThreshold int
	WindowSize              int
}

type HealthSnapshot struct {
	ProviderID       ProviderID
	Status           HealthStatus
	FailureCount     int
	SuccessCount     int
	WindowSize       int
	NextProbeAt      time.Time
	LastCheckAt      time.Time
	LastFailureAt    time.Time
	LastSuccessAt    time.Time
	LastErrorMessage string
}
