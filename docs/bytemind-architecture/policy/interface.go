//go:build ignore

package policy

import (
	"context"

	"bytemind/internal/core"
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

type DecisionStage string

const (
	StageHardDeny      DecisionStage = "hard_deny"
	StageExplicitDeny  DecisionStage = "explicit_deny"
	StageRiskRule      DecisionStage = "risk_rule"
	StageExplicitAllow DecisionStage = "explicit_allow"
	StageModeDefault   DecisionStage = "mode_default"
	StageFallbackAsk   DecisionStage = "fallback_ask"
)

type ToolPolicy struct {
	AllowedTools []string
	DeniedTools  []string
}

type PathCommandPolicy struct {
	AllowedWritePaths []string
	DeniedWritePaths  []string
	AllowedCommands   []string
	DeniedCommands    []string
}

type PermissionRequest struct {
	SessionID   core.SessionID
	TaskID      core.TaskID
	TraceID     core.TraceID
	SessionMode core.SessionMode
	ToolName    string
	Operation   OperationKind
	TargetPaths []string
	Command     string
	Arguments   []string
	RequestedBy string
	ToolPolicy  ToolPolicy
	PathPolicy  PathCommandPolicy
	Metadata    map[string]string
}

type PermissionDecision struct {
	Decision             core.Decision
	ReasonCode           string
	RiskLevel            core.RiskLevel
	Stage                DecisionStage
	RequireUserConfirm   bool
	CanBypassWithSession bool
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
	Stage() DecisionStage
	Evaluate(ctx context.Context, req PermissionRequest) (PermissionDecision, bool, error)
}

type RuleSet interface {
	Register(ctx context.Context, rule Rule) error
	List(ctx context.Context) ([]Rule, error)
}

type PriorityResolver interface {
	Order() []DecisionStage
	Resolve(ctx context.Context, candidates []PermissionDecision) (PermissionDecision, error)
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
