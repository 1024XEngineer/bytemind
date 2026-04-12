package context

import (
	stdctx "context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

type ErrorCode string

const (
	ErrCodeInvalidInput      ErrorCode = "invalid_input"
	ErrCodeBudgetExceeded    ErrorCode = "budget_exceeded"
	ErrCodeCompactionFailed  ErrorCode = "compaction_failed"
	ErrCodeInvariantViolated ErrorCode = "invariant_violated"
)

type Message struct {
	Role       core.Role
	Content    string
	Name       string
	ToolCallID string
	CreatedAt  time.Time
}

type ToolDescriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type RuntimeHint struct {
	ActiveTaskID core.TaskID
	TaskStatus   core.TaskStatus
	Metadata     map[string]string
}

type ProviderLimit struct {
	ModelID         string
	MaxInputTokens  int
	MaxOutputTokens int
}

type BuildRequest struct {
	SessionID      core.SessionID
	UserInput      string
	History        []Message
	SystemPrompts  []Message
	Tools          []ToolDescriptor
	RuntimeHints   []RuntimeHint
	ProviderLimits ProviderLimit
	Metadata       map[string]string
}

type BuildResult struct {
	Messages       []Message
	Tools          []ToolDescriptor
	InputTokens    int
	OutputTokens   int
	UsageRatio     float64
	CompactApplied bool
	CompactMode    string
}

type BuildEventType string

const (
	BuildEventStart   BuildEventType = "start"
	BuildEventPlan    BuildEventType = "budget_plan"
	BuildEventCompact BuildEventType = "compact"
	BuildEventResult  BuildEventType = "result"
	BuildEventError   BuildEventType = "error"
)

type BuildEvent struct {
	Type      BuildEventType
	EventID   string
	Payload   json.RawMessage
	ErrorCode string
	Timestamp time.Time
}

type BudgetPlan struct {
	InputTokens       int
	MaxInputTokens    int
	UsageRatio        float64
	NeedCompact       bool
	CompactMode       string
	PreserveToolPairs bool
}

type Builder interface {
	Build(ctx stdctx.Context, req BuildRequest) (BuildResult, error)
	BuildStream(ctx stdctx.Context, req BuildRequest) (<-chan BuildEvent, error)
}

type Budgeter interface {
	Plan(ctx stdctx.Context, req BuildRequest) (BudgetPlan, error)
}

type Compactor interface {
	Compact(ctx stdctx.Context, history []Message, mode string, preserveToolPairs bool) ([]Message, error)
}

type InvariantChecker interface {
	Check(ctx stdctx.Context, messages []Message) error
}

