package policy

import (
	"context"

	"bytemind/internal/core"
)

type PermissionDecision struct {
	Decision   core.Decision
	ReasonCode string
	RiskLevel  core.RiskLevel
}

type SessionMode string

const (
	ModeDefault           SessionMode = "default"
	ModeAcceptEdits       SessionMode = "acceptEdits"
	ModeBypassPermissions SessionMode = "bypassPermissions"
	ModePlan              SessionMode = "plan"
)

type OperationKind string

const (
	OpRead   OperationKind = "read"
	OpWrite  OperationKind = "write"
	OpExec   OperationKind = "exec"
	OpNet    OperationKind = "network"
	OpSpawn  OperationKind = "spawn_agent"
	OpCustom OperationKind = "custom"
)

type PermissionRequest struct {
	SessionID   core.SessionID
	TaskID      core.TaskID
	SessionMode SessionMode
	ToolName    string
	Operation   OperationKind
	TargetPaths []string
	Command     string
	Arguments   []string
	RequestedBy string
	Metadata    map[string]string
}

type ErrorCode string

const (
	ErrCodeInvalidRequest ErrorCode = "invalid_request"
	ErrCodeRuleConflict   ErrorCode = "rule_conflict"
	ErrCodeRuleNotFound   ErrorCode = "rule_not_found"
)

type Engine interface {
	Evaluate(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

type Rule interface {
	Name() string
	Priority() int
	Evaluate(ctx context.Context, req PermissionRequest) (PermissionDecision, bool, error)
}

type RuleSet interface {
	Register(ctx context.Context, rule Rule) error
	List(ctx context.Context) ([]Rule, error)
}

type PathGuard interface {
	CheckPaths(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

type CommandGuard interface {
	CheckCommand(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

type SensitiveFileGuard interface {
	CheckSensitiveRead(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

