package policy

import (
	"strings"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type ToolAccessInput struct {
	ToolName string
	Allowed  map[string]struct{}
	Denied   map[string]struct{}
}

type ToolAccessDecision struct {
	Decision corepkg.Decision
	Reason   string
}

// DecideToolAccess evaluates whether a tool can run under active allow/deny constraints.
func DecideToolAccess(in ToolAccessInput) ToolAccessDecision {
	eval := Evaluate(EvaluateInput{
		ToolName:          strings.TrimSpace(in.ToolName),
		Allowed:           in.Allowed,
		Denied:            in.Denied,
		SkipRuntimeChecks: true,
	})
	if eval.MainDecision == MainDecisionAllow {
		return ToolAccessDecision{Decision: corepkg.DecisionAllow, Reason: eval.MainReason}
	}
	return ToolAccessDecision{Decision: corepkg.DecisionDeny, Reason: eval.MainReason}
}
