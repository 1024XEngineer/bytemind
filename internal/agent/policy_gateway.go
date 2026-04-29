package agent

import (
	"context"
	"strings"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

const (
	policyReasonHardDeny      = "hard_deny"
	policyReasonExplicitDeny  = "explicit_deny"
	policyReasonRiskRule      = "risk_rule"
	policyReasonSandboxGuard  = "sandbox_guard"
	policyReasonExplicitAllow = "explicit_allow"
	policyReasonModeDefault   = "mode_default"
	policyReasonFallbackAsk   = "fallback_ask"
)

type ToolDecisionInput struct {
	ToolName             string
	ToolArguments        string
	AllowedTools         map[string]struct{}
	DeniedTools          map[string]struct{}
	ApprovalPolicy       string
	SafetyClass          tools.SafetyClass
	SandboxEnabled       bool
	SandboxMode          string
	SandboxBackend       string
	SandboxCapability    string
	SandboxRequiredCapab bool
	SandboxShellNetwork  bool
	SandboxWorkerNetwork bool
}

type ToolDecision struct {
	Decision   corepkg.Decision
	ReasonCode string
	Reason     string
	RiskLevel  corepkg.RiskLevel
}

type PolicyGateway interface {
	DecideTool(context.Context, ToolDecisionInput) (ToolDecision, error)
}

type defaultPolicyGateway struct{}

func NewDefaultPolicyGateway() PolicyGateway {
	return defaultPolicyGateway{}
}

func (defaultPolicyGateway) DecideTool(_ context.Context, in ToolDecisionInput) (ToolDecision, error) {
	name := strings.TrimSpace(in.ToolName)
	if name == "" {
		return ToolDecision{
			Decision:   corepkg.DecisionDeny,
			ReasonCode: policyReasonHardDeny,
			Reason:     "tool name is empty",
			RiskLevel:  corepkg.RiskHigh,
		}, nil
	}

	if len(in.DeniedTools) > 0 {
		if _, blocked := in.DeniedTools[name]; blocked {
			return ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonExplicitDeny,
				Reason:     "tool is in active denylist",
				RiskLevel:  toRiskLevel(in.SafetyClass),
			}, nil
		}
	}

	allowlistActive := len(in.AllowedTools) > 0
	if allowlistActive {
		if _, allowed := in.AllowedTools[name]; !allowed {
			return ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonExplicitDeny,
				Reason:     "tool is not in active allowlist",
				RiskLevel:  toRiskLevel(in.SafetyClass),
			}, nil
		}
	}

	if decision, ok := decideBySandboxGuard(in); ok {
		return decision, nil
	}

	if decision, ok := decideByRiskRule(in); ok {
		return decision, nil
	}

	if allowlistActive {
		return ToolDecision{
			Decision:   corepkg.DecisionAllow,
			ReasonCode: policyReasonExplicitAllow,
			Reason:     "tool is explicitly allowed by active allowlist",
			RiskLevel:  toRiskLevel(in.SafetyClass),
		}, nil
	}

	approval := normalizeApprovalPolicy(in.ApprovalPolicy)
	if approval == "" {
		return ToolDecision{
			Decision:   corepkg.DecisionAsk,
			ReasonCode: policyReasonFallbackAsk,
			Reason:     "approval policy is unknown; fallback to ask",
			RiskLevel:  toRiskLevel(in.SafetyClass),
		}, nil
	}

	return ToolDecision{
		Decision:   corepkg.DecisionAllow,
		ReasonCode: policyReasonModeDefault,
		Reason:     "tool is allowed by mode default policy",
		RiskLevel:  toRiskLevel(in.SafetyClass),
	}, nil
}

func decideBySandboxGuard(in ToolDecisionInput) (ToolDecision, bool) {
	if !in.SandboxEnabled {
		return ToolDecision{}, false
	}
	mode := strings.ToLower(strings.TrimSpace(in.SandboxMode))
	if mode != "required" {
		return ToolDecision{}, false
	}
	name := strings.TrimSpace(in.ToolName)
	switch name {
	case "web_fetch", "web_search":
		if in.SandboxWorkerNetwork {
			return ToolDecision{}, false
		}
		backend := sandboxBackendDisplayName(in.SandboxBackend)
		return ToolDecision{
			Decision:   corepkg.DecisionDeny,
			ReasonCode: policyReasonSandboxGuard,
			Reason:     "system sandbox required mode cannot run web_* tools because backend " + backend + " worker network isolation is unavailable",
			RiskLevel:  toRiskLevel(in.SafetyClass),
		}, true
	case "run_shell":
		if !runShellArgumentsRequireNetworkIsolation(in.ToolArguments) {
			return ToolDecision{}, false
		}
		if in.SandboxShellNetwork {
			return ToolDecision{}, false
		}
		backend := sandboxBackendDisplayName(in.SandboxBackend)
		return ToolDecision{
			Decision:   corepkg.DecisionDeny,
			ReasonCode: policyReasonSandboxGuard,
			Reason:     "system sandbox required mode cannot run network-targeted run_shell because backend " + backend + " shell network isolation is unavailable",
			RiskLevel:  toRiskLevel(in.SafetyClass),
		}, true
	default:
		return ToolDecision{}, false
	}
}

func runShellArgumentsRequireNetworkIsolation(raw string) bool {
	return tools.RunShellArgumentsContainNetworkTarget(raw)
}

func sandboxBackendDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "<unknown>"
	}
	return value
}

func decideByRiskRule(in ToolDecisionInput) (ToolDecision, bool) {
	approval := normalizeApprovalPolicy(in.ApprovalPolicy)
	if approval == "" {
		return ToolDecision{}, false
	}

	risk := toRiskLevel(in.SafetyClass)
	if risk != corepkg.RiskHigh {
		return ToolDecision{}, false
	}

	switch approval {
	case "never":
		return ToolDecision{
			Decision:   corepkg.DecisionDeny,
			ReasonCode: policyReasonRiskRule,
			Reason:     "high-risk tools are blocked under never approval policy",
			RiskLevel:  risk,
		}, true
	case "on-request":
		return ToolDecision{
			Decision:   corepkg.DecisionAsk,
			ReasonCode: policyReasonRiskRule,
			Reason:     "high-risk tools require explicit approval",
			RiskLevel:  risk,
		}, true
	default:
		return ToolDecision{}, false
	}
}

func normalizeApprovalPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "on-request"
	case "always", "on-request", "never":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func toRiskLevel(class tools.SafetyClass) corepkg.RiskLevel {
	switch class {
	case tools.SafetyClassSafe:
		return corepkg.RiskLow
	case tools.SafetyClassModerate:
		return corepkg.RiskMedium
	case tools.SafetyClassSensitive, tools.SafetyClassDestructive:
		return corepkg.RiskHigh
	default:
		return corepkg.RiskMedium
	}
}
