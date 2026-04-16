package agent

import (
	"context"
	"testing"

	corepkg "bytemind/internal/core"
	"bytemind/internal/tools"
)

func TestDefaultPolicyGatewayDecisionPriority(t *testing.T) {
	gateway := NewDefaultPolicyGateway()

	tests := []struct {
		name string
		in   ToolDecisionInput
		want ToolDecision
	}{
		{
			name: "hard_deny_empty_name",
			in:   ToolDecisionInput{ToolName: "", ApprovalPolicy: "always"},
			want: ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonHardDeny,
				RiskLevel:  corepkg.RiskHigh,
			},
		},
		{
			name: "explicit_deny_from_denylist",
			in: ToolDecisionInput{
				ToolName:       "run_shell",
				DeniedTools:    map[string]struct{}{"run_shell": {}},
				ApprovalPolicy: "always",
				SafetyClass:    tools.SafetyClassSensitive,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonExplicitDeny,
				RiskLevel:  corepkg.RiskHigh,
			},
		},
		{
			name: "risk_rule_ask_for_high_risk_on_request",
			in: ToolDecisionInput{
				ToolName:       "run_shell",
				ApprovalPolicy: "on-request",
				SafetyClass:    tools.SafetyClassSensitive,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionAsk,
				ReasonCode: policyReasonRiskRule,
				RiskLevel:  corepkg.RiskHigh,
			},
		},
		{
			name: "risk_rule_deny_for_high_risk_never",
			in: ToolDecisionInput{
				ToolName:       "apply_patch",
				ApprovalPolicy: "never",
				SafetyClass:    tools.SafetyClassDestructive,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonRiskRule,
				RiskLevel:  corepkg.RiskHigh,
			},
		},
		{
			name: "explicit_allow_from_allowlist",
			in: ToolDecisionInput{
				ToolName:       "read_file",
				AllowedTools:   map[string]struct{}{"read_file": {}},
				ApprovalPolicy: "always",
				SafetyClass:    tools.SafetyClassSafe,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionAllow,
				ReasonCode: policyReasonExplicitAllow,
				RiskLevel:  corepkg.RiskLow,
			},
		},
		{
			name: "mode_default_without_lists",
			in: ToolDecisionInput{
				ToolName:       "read_file",
				ApprovalPolicy: "always",
				SafetyClass:    tools.SafetyClassSafe,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionAllow,
				ReasonCode: policyReasonModeDefault,
				RiskLevel:  corepkg.RiskLow,
			},
		},
		{
			name: "fallback_ask_unknown_approval_policy",
			in: ToolDecisionInput{
				ToolName:       "read_file",
				ApprovalPolicy: "unknown-policy",
				SafetyClass:    tools.SafetyClassSafe,
			},
			want: ToolDecision{
				Decision:   corepkg.DecisionAsk,
				ReasonCode: policyReasonFallbackAsk,
				RiskLevel:  corepkg.RiskLow,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.DecideTool(context.Background(), tt.in)
			if err != nil {
				t.Fatalf("DecideTool failed: %v", err)
			}
			if got.Decision != tt.want.Decision {
				t.Fatalf("Decision mismatch: got %s want %s", got.Decision, tt.want.Decision)
			}
			if got.ReasonCode != tt.want.ReasonCode {
				t.Fatalf("ReasonCode mismatch: got %s want %s", got.ReasonCode, tt.want.ReasonCode)
			}
			if got.RiskLevel != tt.want.RiskLevel {
				t.Fatalf("RiskLevel mismatch: got %s want %s", got.RiskLevel, tt.want.RiskLevel)
			}
			if got.Reason == "" {
				t.Fatal("expected non-empty reason")
			}
		})
	}
}
