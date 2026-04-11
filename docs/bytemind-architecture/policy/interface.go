package policy

import "context"

// Engine evaluates permission decisions.
type Engine interface {
	Decide(ctx context.Context, input DecisionInput) (DecisionOutput, error)
}

// DecisionInput is the policy evaluation input.
type DecisionInput struct {
	ToolName string
	Path     string
	Command  string
}

// DecisionOutput is the policy evaluation output.
type DecisionOutput struct {
	Decision string
	Reason   string
	Risk     string
}
