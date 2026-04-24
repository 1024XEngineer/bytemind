package agent

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func shouldRepairBuildHandoffTurn(runMode planpkg.AgentMode, state planpkg.State, intent assistantTurnIntent, reply llm.Message, messages []llm.Message) bool {
	if runMode != planpkg.ModeBuild || len(reply.ToolCalls) > 0 {
		return false
	}

	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return false
	}
	phase := planpkg.NormalizePhase(string(state.Phase))
	if phase != planpkg.PhaseExecuting && !planpkg.CanStartExecution(state) {
		return false
	}

	latestUser := latestUserMessageText(messages)
	if !looksLikeExecutionHandoffInput(latestUser) {
		return false
	}

	text := strings.TrimSpace(reply.Content)
	if text == "" || intent == turnIntentContinueWork {
		return false
	}
	if looksLikeRedundantBuildHandoffBlocker(text) {
		return true
	}
	return false
}

func buildBuildHandoffRepairInstruction(state planpkg.State, reply llm.Message, latestUser string, attempt, maxAttempts int) string {
	state = planpkg.NormalizeState(state)
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)
	latestUser = strings.TrimSpace(latestUser)
	if latestUser == "" {
		latestUser = "(empty user text)"
	}
	nextStep := strings.TrimSpace(state.NextAction)
	if nextStep == "" {
		nextStep = planpkg.DefaultNextAction(state)
	}

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn responded to an execution handoff after the session had already switched to build mode.
Attempt %d/%d.

Latest user handoff input:
%s

Reply text preview:
%s

Current mode is build and the plan handoff is already approved. For this next turn:
1) Do not ask the user to send continue execution/start execution again.
2) Do not claim the session is still in plan mode, stuck in plan confirmation, or limited to a plan-mode read-only shell policy.
3) Start from the current plan baseline and the next execution step: %s
4) Emit structured tool calls in this turn unless a real missing requirement blocks execution.
5) If an action genuinely needs approval, rely on the tool approval flow instead of asking the user to switch modes again.`,
		attempt,
		maxAttempts,
		latestUser,
		preview,
		nextStep,
	))
}

func latestUserMessageText(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != llm.RoleUser {
			continue
		}
		text := strings.TrimSpace(msg.Text())
		if text != "" {
			return text
		}
	}
	return ""
}

func looksLikeExecutionHandoffInput(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return containsAnyToken(normalized,
		"start execution",
		"continue execution",
		"start build",
		"begin execution",
		"resume execution",
		"开始执行",
		"继续执行",
		"按计划执行",
	)
}

func looksLikeRedundantBuildHandoffBlocker(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if containsAnyToken(normalized,
		"plan confirmation",
		"still in plan",
		"still stuck in plan",
		"read-only",
		"run_shell",
		"strict read-only allowlist",
		"switch to build",
		"switch the ui",
		"toggle the ui",
		"send continue execution",
		"send start execution",
		"计划确认",
		"仍停在计划",
		"只读",
		"切到 build",
		"切换到 build",
		"再发 continue execution",
		"再发 start execution",
	) {
		return true
	}
	return hasAskUserSignal(normalized) &&
		containsAnyToken(normalized, "continue execution", "start execution", "switch to build", "build mode", "继续执行", "开始执行")
}
