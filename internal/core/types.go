package core

// SessionID identifies a persisted conversation session.
type SessionID string

// TaskID identifies an asynchronous runtime task or sub-agent run.
type TaskID string

// EventID uniquely identifies a persisted event record.
type EventID string

// TraceID links events across modules for a single request flow.
type TraceID string

// Role normalizes message authorship across provider/session/tool boundaries.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Decision is the normalized permission decision.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// RiskLevel classifies operation risk for policy and audit.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// TaskStatus is the normalized runtime task status.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskKilled    TaskStatus = "killed"
)

// SessionMode is the normalized interaction mode at session scope.
type SessionMode string

const (
	SessionModeDefault           SessionMode = "default"
	SessionModeAcceptEdits       SessionMode = "acceptEdits"
	SessionModeBypassPermissions SessionMode = "bypassPermissions"
	SessionModePlan              SessionMode = "plan"
)

// EventMeta carries cross-module event identity metadata.
type EventMeta struct {
	EventID   EventID
	SessionID SessionID
	TaskID    TaskID
	TraceID   TraceID
}

// SemanticError is the minimal, testable cross-module error contract.
type SemanticError interface {
	error
	Code() string
	Retryable() bool
}
