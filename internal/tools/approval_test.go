package tools

import "testing"

func TestApprovalDecisionHelpers(t *testing.T) {
	cases := []struct {
		name              string
		decision          ApprovalDecision
		approved          bool
		reuseSameTool     bool
		reuseAllTools     bool
		normalizedOutcome ApprovalDisposition
	}{
		{
			name:              "approve once",
			decision:          ApprovalDecision{Disposition: ApprovalApproveOnce},
			approved:          true,
			reuseSameTool:     false,
			reuseAllTools:     false,
			normalizedOutcome: ApprovalApproveOnce,
		},
		{
			name:              "approve same tool",
			decision:          ApprovalDecision{Disposition: ApprovalApproveSameToolSession},
			approved:          true,
			reuseSameTool:     true,
			reuseAllTools:     false,
			normalizedOutcome: ApprovalApproveSameToolSession,
		},
		{
			name:              "approve all tools",
			decision:          ApprovalDecision{Disposition: ApprovalApproveAllSession},
			approved:          true,
			reuseSameTool:     true,
			reuseAllTools:     true,
			normalizedOutcome: ApprovalApproveAllSession,
		},
		{
			name:              "deny",
			decision:          ApprovalDecision{Disposition: ApprovalDeny},
			approved:          false,
			reuseSameTool:     false,
			reuseAllTools:     false,
			normalizedOutcome: ApprovalDeny,
		},
		{
			name:              "unknown disposition normalizes to deny",
			decision:          ApprovalDecision{Disposition: ApprovalDisposition("custom")},
			approved:          false,
			reuseSameTool:     false,
			reuseAllTools:     false,
			normalizedOutcome: ApprovalDeny,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.decision.Approved(); got != tc.approved {
				t.Fatalf("expected Approved=%v, got %v", tc.approved, got)
			}
			if got := tc.decision.ReusableForSameTool(); got != tc.reuseSameTool {
				t.Fatalf("expected ReusableForSameTool=%v, got %v", tc.reuseSameTool, got)
			}
			if got := tc.decision.ReusableForAllTools(); got != tc.reuseAllTools {
				t.Fatalf("expected ReusableForAllTools=%v, got %v", tc.reuseAllTools, got)
			}
			if got := NormalizeApprovalDecision(tc.decision).Disposition; got != tc.normalizedOutcome {
				t.Fatalf("expected normalized disposition %q, got %q", tc.normalizedOutcome, got)
			}
		})
	}
}
