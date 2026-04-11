package policy

import "context"

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

type PermissionDecision struct {
	Decision   Decision
	ReasonCode string
	RiskLevel  RiskLevel
}

// Engine evaluates permission decisions.
type Engine interface {
	Decide(ctx context.Context, input DecisionInput) (PermissionDecision, error)
}

// DecisionInput is the policy evaluation input.
type DecisionInput struct {
	Mode              string
	ToolName          string
	Path              string
	Command           string
	AllowedTools      []string
	DeniedTools       []string
	AllowedWritePaths []string
	DeniedWritePaths  []string
	AllowedCommands   []string
	DeniedCommands    []string
}
