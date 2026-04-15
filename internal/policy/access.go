package policy

import (
	"strings"

	corepkg "bytemind/internal/core"
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
	name := strings.TrimSpace(in.ToolName)
	if name == "" {
		return ToolAccessDecision{Decision: corepkg.DecisionDeny, Reason: "tool name is empty"}
	}
	if len(in.Allowed) > 0 {
		if _, ok := in.Allowed[name]; !ok {
			return ToolAccessDecision{Decision: corepkg.DecisionDeny, Reason: "tool is not in active allowlist"}
		}
	}
	if len(in.Denied) > 0 {
		if _, blocked := in.Denied[name]; blocked {
			return ToolAccessDecision{Decision: corepkg.DecisionDeny, Reason: "tool is in active denylist"}
		}
	}
	return ToolAccessDecision{Decision: corepkg.DecisionAllow}
}
