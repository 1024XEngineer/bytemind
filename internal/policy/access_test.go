package policy

import (
	"testing"

	corepkg "bytemind/internal/core"
)

func TestDecideToolAccessAllowlist(t *testing.T) {
	allowed := map[string]struct{}{"read_file": {}}
	decision := DecideToolAccess(ToolAccessInput{ToolName: "read_file", Allowed: allowed})
	if decision.Decision != corepkg.DecisionAllow {
		t.Fatalf("expected allow, got %q (%s)", decision.Decision, decision.Reason)
	}
	decision = DecideToolAccess(ToolAccessInput{ToolName: "run_shell", Allowed: allowed})
	if decision.Decision != corepkg.DecisionDeny {
		t.Fatal("expected deny when tool not in allowlist")
	}
}

func TestDecideToolAccessAllowlistRunShellDoesNotRequireCommand(t *testing.T) {
	allowed := map[string]struct{}{"run_shell": {}}
	decision := DecideToolAccess(ToolAccessInput{ToolName: "run_shell", Allowed: allowed})
	if decision.Decision != corepkg.DecisionAllow {
		t.Fatalf("expected allow for allowlisted run_shell, got %q (%s)", decision.Decision, decision.Reason)
	}
}

func TestDecideToolAccessDenylist(t *testing.T) {
	denied := map[string]struct{}{"run_shell": {}}
	decision := DecideToolAccess(ToolAccessInput{ToolName: "run_shell", Denied: denied})
	if decision.Decision != corepkg.DecisionDeny {
		t.Fatal("expected deny when tool in denylist")
	}
}

func TestDecideToolAccessEmptyToolName(t *testing.T) {
	decision := DecideToolAccess(ToolAccessInput{})
	if decision.Decision != corepkg.DecisionDeny {
		t.Fatal("expected deny for empty tool name")
	}
}
