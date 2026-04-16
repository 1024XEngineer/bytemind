package policy

import (
	"encoding/json"
	"strings"

	planpkg "bytemind/internal/plan"
)

type MainDecision string

const (
	MainDecisionAllow    MainDecision = "allow"
	MainDecisionDeny     MainDecision = "deny"
	MainDecisionEscalate MainDecision = "escalate"
)

type PromptHintDecision string

const (
	PromptHintDecisionHint PromptHintDecision = "hint"
	PromptHintDecisionNone PromptHintDecision = "none"
)

type DecisionSource string

const (
	DecisionSourceRuntime DecisionSource = "runtime"
	DecisionSourceSkill   DecisionSource = "skill"
)

type ReasonCode string

const (
	ReasonInvalidToolName                 ReasonCode = "invalid_tool_name"
	ReasonToolDeniedByDenylist            ReasonCode = "tool_denied_by_denylist"
	ReasonToolNotInAllowlist              ReasonCode = "tool_not_in_allowlist"
	ReasonToolAllowed                     ReasonCode = "tool_allowed"
	ReasonDestructiveToolRequiresApproval ReasonCode = "destructive_tool_requires_approval"
	ReasonDangerousCommandBlocked         ReasonCode = "dangerous_command_blocked"
	ReasonPlanModeToolBlocked             ReasonCode = "plan_mode_tool_blocked"
	ReasonWebLookupHintInjected           ReasonCode = "web_lookup_hint_injected"
	ReasonPromptHintSkipped               ReasonCode = "prompt_hint_skipped"
	ReasonPolicyEvaluationError           ReasonCode = "policy_evaluation_error"
)

type PromptHintResult struct {
	Decision    PromptHintDecision
	ReasonCode  ReasonCode
	Reason      string
	Instruction string
}

type Evaluation struct {
	MainDecision   MainDecision
	MainReasonCode ReasonCode
	MainReason     string
	MainSource     DecisionSource
	PromptHint     PromptHintResult
}

type ToolSpec struct {
	Name        string
	Destructive bool
}

type EvaluateInput struct {
	ToolName          string
	ToolSpec          ToolSpec
	ToolArgs          json.RawMessage
	ShellCommand      string
	Allowed           map[string]struct{}
	Denied            map[string]struct{}
	Mode              planpkg.AgentMode
	ApprovalPolicy    string
	UserInput         string
	SkipRuntimeChecks bool
}

// Evaluate returns a unified policy decision: one executable main decision and
// one optional prompt-hint sub decision.
func Evaluate(in EvaluateInput) Evaluation {
	out := Evaluation{
		MainDecision:   MainDecisionAllow,
		MainReasonCode: ReasonToolAllowed,
		MainReason:     "tool is allowed",
		MainSource:     DecisionSourceRuntime,
		PromptHint:     EvaluatePromptHint(in.UserInput),
	}

	toolName := strings.TrimSpace(in.ToolName)
	if toolName == "" {
		toolName = strings.TrimSpace(in.ToolSpec.Name)
	}
	if toolName == "" {
		out.MainDecision = MainDecisionDeny
		out.MainReasonCode = ReasonInvalidToolName
		out.MainReason = "tool name is empty"
		return out
	}

	if !in.SkipRuntimeChecks && strings.EqualFold(toolName, "run_shell") {
		shellDecision := evaluateRunShellPolicy(in)
		if shellDecision.MainDecision != MainDecisionAllow {
			return shellDecision
		}
	}

	if len(in.Denied) > 0 {
		if _, blocked := in.Denied[toolName]; blocked {
			out.MainDecision = MainDecisionDeny
			out.MainReasonCode = ReasonToolDeniedByDenylist
			out.MainReason = "tool is in active denylist"
			out.MainSource = DecisionSourceSkill
			return out
		}
	}

	if len(in.Allowed) > 0 {
		if _, ok := in.Allowed[toolName]; !ok {
			out.MainDecision = MainDecisionDeny
			out.MainReasonCode = ReasonToolNotInAllowlist
			out.MainReason = "tool is not in active allowlist"
			out.MainSource = DecisionSourceSkill
			return out
		}
	}

	if in.ToolSpec.Destructive && needsApproval(in.ApprovalPolicy) {
		out.MainDecision = MainDecisionEscalate
		out.MainReasonCode = ReasonDestructiveToolRequiresApproval
		out.MainReason = "destructive tool requires approval"
		return out
	}

	return out
}

func needsApproval(policy string) bool {
	switch strings.TrimSpace(policy) {
	case "never":
		return false
	default:
		return true
	}
}

func EvaluatePromptHint(userInput string) PromptHintResult {
	instruction := ExplicitWebLookupInstruction(userInput)
	if strings.TrimSpace(instruction) == "" {
		return PromptHintResult{
			Decision:   PromptHintDecisionNone,
			ReasonCode: ReasonPromptHintSkipped,
			Reason:     "no explicit web lookup request detected",
		}
	}
	return PromptHintResult{
		Decision:    PromptHintDecisionHint,
		ReasonCode:  ReasonWebLookupHintInjected,
		Reason:      "explicit web lookup request detected",
		Instruction: instruction,
	}
}

func evaluateRunShellPolicy(in EvaluateInput) Evaluation {
	out := Evaluation{
		MainDecision:   MainDecisionAllow,
		MainReasonCode: ReasonToolAllowed,
		MainReason:     "tool is allowed",
		MainSource:     DecisionSourceRuntime,
		PromptHint:     EvaluatePromptHint(in.UserInput),
	}
	command := strings.TrimSpace(in.ShellCommand)
	if command == "" {
		command = strings.TrimSpace(extractRunShellCommand(in.ToolArgs))
	}
	if command == "" {
		out.MainDecision = MainDecisionDeny
		out.MainReasonCode = ReasonPolicyEvaluationError
		out.MainReason = "shell command is empty"
		return out
	}

	mode := planpkg.NormalizeMode(string(in.Mode))
	if mode == planpkg.ModePlan {
		if !IsPlanSafeShellCommand(command) {
			out.MainDecision = MainDecisionDeny
			out.MainReasonCode = ReasonPlanModeToolBlocked
			out.MainReason = "shell command is unavailable in plan mode unless it matches the strict read-only allowlist"
			return out
		}
		return out
	}

	assessment := AssessShellCommand(command)
	if assessment.Risk == ShellRiskBlocked {
		out.MainDecision = MainDecisionDeny
		out.MainReasonCode = ReasonDangerousCommandBlocked
		out.MainReason = strings.TrimSpace(assessment.Reason)
		if out.MainReason == "" {
			out.MainReason = "blocked dangerous shell command"
		}
		return out
	}

	switch strings.TrimSpace(in.ApprovalPolicy) {
	case "always":
		out.MainDecision = MainDecisionEscalate
		out.MainReasonCode = ReasonDestructiveToolRequiresApproval
		out.MainReason = "shell command requires approval under policy always"
		return out
	case "never":
		return out
	default:
		if assessment.Risk == ShellRiskApproval {
			out.MainDecision = MainDecisionEscalate
			out.MainReasonCode = ReasonDestructiveToolRequiresApproval
			out.MainReason = strings.TrimSpace(assessment.Reason)
			if out.MainReason == "" {
				out.MainReason = "shell command requires approval"
			}
			return out
		}
		return out
	}
}

func extractRunShellCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Command)
}
