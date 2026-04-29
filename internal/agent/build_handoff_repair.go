package agent

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
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
	return looksLikeRedundantBuildHandoffBlocker(text) || looksLikeExecutionHandoffAcknowledgement(text)
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
		"\u5f00\u59cb\u6267\u884c",
		"\u7ee7\u7eed\u6267\u884c",
		"\u6309\u8ba1\u5212\u6267\u884c",
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
		"\u8ba1\u5212\u786e\u8ba4",
		"\u4ecd\u505c\u5728\u8ba1\u5212",
		"\u53ea\u8bfb",
		"\u5207\u5230 build",
		"\u5207\u6362\u5230 build",
		"\u518d\u53d1 continue execution",
		"\u518d\u53d1 start execution",
	) {
		return true
	}
	return hasAskUserSignal(normalized) &&
		containsAnyToken(normalized,
			"continue execution",
			"start execution",
			"switch to build",
			"build mode",
			"\u7ee7\u7eed\u6267\u884c",
			"\u5f00\u59cb\u6267\u884c",
		)
}

func looksLikeExecutionHandoffAcknowledgement(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if containsAnyToken(normalized,
		"need you",
		"need your",
		"please confirm",
		"confirm whether",
		"confirm if",
		"do you accept",
		"accept me",
		"reply with",
		"回复",
		"请确认",
		"是否接受",
		"是否同意",
		"我需要你确认",
		"请先确认",
	) {
		return true
	}
	if hasAskUserSignal(normalized) || containsAnyToken(normalized,
		"missing ",
		"need ",
		"blocked",
		"cannot",
		"can't",
		"unable",
		"\u7f3a\u5c11",
		"\u9700\u8981",
		"\u963b\u585e",
		"\u65e0\u6cd5",
	) {
		return false
	}
	if strings.Count(normalized, "\n") > 4 || len([]rune(normalized)) > 160 {
		return false
	}
	return containsAnyToken(normalized,
		"started execution",
		"starting execution",
		"start execution",
		"beginning execution",
		"begin execution",
		"starting now",
		"\u5df2\u5f00\u59cb\u6267\u884c",
		"\u5f00\u59cb\u6267\u884c",
		"\u51c6\u5907\u5f00\u5de5",
		"\u9a6c\u4e0a\u5f00\u59cb",
		"\u5f00\u59cb\u52a8\u624b",
	)
}
