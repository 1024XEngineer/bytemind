package tools

import (
	"context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage, tctx ToolUseContext) (<-chan ToolEvent, error)
}

type SideEffectLevel string

const (
	SideEffectNone  SideEffectLevel = "none"
	SideEffectRead  SideEffectLevel = "read"
	SideEffectWrite SideEffectLevel = "write"
	SideEffectExec  SideEffectLevel = "exec"
)

type IdempotencyLevel string

const (
	IdempotencyStrong  IdempotencyLevel = "strong"
	IdempotencyWeak    IdempotencyLevel = "weak"
	IdempotencyUnknown IdempotencyLevel = "unknown"
)

type ToolMetadata struct {
	Layer            string
	SideEffectLevel  SideEffectLevel
	IdempotencyLevel IdempotencyLevel
	DefaultTimeout   time.Duration
	MaxRetries       int
}

type ToolMetadataProvider interface {
	Metadata() ToolMetadata
}

type ToolUseContext struct {
	SessionID core.SessionID
	TaskID    core.TaskID
	Workspace string
	Invoker   string
	Attempt   int
	Deadline  time.Time
	Metadata  map[string]string
}

type ToolEventType string

const (
	ToolEventStart  ToolEventType = "start"
	ToolEventChunk  ToolEventType = "chunk"
	ToolEventResult ToolEventType = "result"
	ToolEventError  ToolEventType = "error"
)

type ToolEvent struct {
	Type      ToolEventType
	ToolName  string
	CallID    string
	EventID   string
	Offset    int64
	Payload   json.RawMessage
	ErrorCode string
	Timestamp time.Time
}

type ErrorCode string

const (
	ErrCodeInvalidArgument ErrorCode = "invalid_argument"
	ErrCodeSchemaViolation ErrorCode = "schema_violation"
	ErrCodeTimeout         ErrorCode = "timeout"
	ErrCodeCanceled        ErrorCode = "canceled"
	ErrCodeExecutionFailed ErrorCode = "execution_failed"
	ErrCodeUnavailable     ErrorCode = "unavailable"
)

type ToolDescriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Metadata    ToolMetadata
}

type Registry interface {
	Register(ctx context.Context, tool Tool) error
	Unregister(ctx context.Context, name string) error
	Get(ctx context.Context, name string) (Tool, bool)
	List(ctx context.Context) ([]ToolDescriptor, error)
}

type Validator interface {
	Validate(ctx context.Context, schema json.RawMessage, args json.RawMessage) error
}

type Executor interface {
	Run(ctx context.Context, tool Tool, args json.RawMessage, tctx ToolUseContext) (<-chan ToolEvent, error)
}

