//go:build ignore

package core

import "time"

type SessionID string
type TaskID string
type EventID string
type TraceID string

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type MessagePartType string

const (
	PartText  MessagePartType = "text"
	PartImage MessagePartType = "image"
	PartFile  MessagePartType = "file"
	PartAudio MessagePartType = "audio"
)

// MessagePart is the shared multimodal payload unit.
// Text part uses Text; media part uses URI/MIMEType/Name.
type MessagePart struct {
	Type     MessagePartType
	Text     string
	URI      string
	MIMEType string
	Name     string
	Metadata map[string]string
}

type SessionMode string

const (
	SessionModeDefault           SessionMode = "default"
	SessionModeAcceptEdits       SessionMode = "acceptEdits"
	SessionModeBypassPermissions SessionMode = "bypassPermissions"
	SessionModePlan              SessionMode = "plan"
)

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskKilled    TaskStatus = "killed"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// EventMeta is the shared envelope carried by every stream event.
type EventMeta struct {
	EventID   EventID
	TraceID   TraceID
	SessionID SessionID
	TaskID    TaskID
	Timestamp time.Time
}

// SemanticError is the shared minimum contract for testable errors.
type SemanticError interface {
	error
	Code() string
	Retryable() bool
}
