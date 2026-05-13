//go:build ignore

package context

import (
	stdctx "context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

const (
	WarningBudgetThreshold  = 0.85
	CriticalBudgetThreshold = 0.95
	ReactiveRetryLimit      = 1
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
	Parts      []core.MessagePart
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

type CompactionMode string

const (
	CompactionWarning  CompactionMode = "warning"
	CompactionCritical CompactionMode = "critical"
	CompactionReactive CompactionMode = "reactive"
)

type BuildRequest struct {
	SessionID      core.SessionID
	TraceID        core.TraceID
	UserInputParts []core.MessagePart
	History        []Message
	SystemPrompts  []Message
	Tools          []ToolDescriptor
	RuntimeHints   []RuntimeHint
	ProviderLimits ProviderLimit
	Metadata       map[string]string
}

type BuildResult struct {
	Messages         []Message
	Tools            []ToolDescriptor
	InputTokens      int
	OutputTokens     int
	UsageRatio       float64
	CompactApplied   bool
	CompactMode      CompactionMode
	ReactiveRetryUse int
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
	Meta      core.EventMeta
	Payload   json.RawMessage
	ErrorCode string
}

type BudgetPlan struct {
	InputTokens       int
	MaxInputTokens    int
	UsageRatio        float64
	NeedCompact       bool
	CompactMode       CompactionMode
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
	Compact(ctx stdctx.Context, history []Message, mode CompactionMode, preserveToolPairs bool) ([]Message, error)
}

type InvariantChecker interface {
	Check(ctx stdctx.Context, messages []Message) error
}
