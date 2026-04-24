package agent

import (
	"fmt"
	"regexp"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

var clarifyChoiceShortcutPattern = regexp.MustCompile(`(?i)(^|[\s/|,，;；:：\-])([a-d]|1|2|3|4)([.)：:\s]|$)`)

func shouldRepairPlanRevisionTurn(runMode planpkg.AgentMode, state planpkg.State, latestUser string, reply llm.Message) bool {
	if runMode != planpkg.ModePlan || len(reply.ToolCalls) > 0 {
		return false
	}

	state = planpkg.NormalizeState(state)
	if !planpkg.CanStartExecution(state) {
		return false
	}

	latestUser = strings.TrimSpace(latestUser)
	if latestUser == "" || !looksLikePlanRevisionInput(latestUser) {
		return false
	}

	return strings.TrimSpace(reply.Content) != ""
}

func buildPlanRevisionRepairInstruction(state planpkg.State, latestUser string, reply llm.Message, attempt, maxAttempts int) string {
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

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn responded to plan-refinement feedback without updating the structured plan first.
Attempt %d/%d.

Latest user refinement input:
%s

Reply text preview:
%s

For this next turn:
1) Call update_plan before finalizing.
2) Merge the new details into the structured plan state, especially summary, implementation_brief, steps, verification, and decision_log when relevant.
3) If the refinement introduces a new unresolved decision, record it in decision_gaps, populate active_choice, and ask only that next question.
4) If the plan remains converged after the update, keep the visible assistant reply to one short acknowledgement only and let the refreshed structured plan carry the detailed content.
5) Do not answer with a standalone design memo or suggestion block outside the updated plan document.`,
		attempt,
		maxAttempts,
		latestUser,
		preview,
	))
}

func shouldRepairPlanClarifyTurn(runMode planpkg.AgentMode, state planpkg.State, intent assistantTurnIntent, reply llm.Message) bool {
	if runMode != planpkg.ModePlan || len(reply.ToolCalls) > 0 {
		return false
	}

	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) || planpkg.HasActiveChoice(state) {
		return false
	}
	if planpkg.CanStartExecution(state) {
		return false
	}

	text := strings.TrimSpace(reply.Content)
	if text == "" {
		return false
	}

	if intent == turnIntentAskUser {
		return planpkg.HasDecisionGaps(state) || looksLikeInlineClarifyChoicePrompt(text)
	}
	return looksLikeInlineClarifyChoicePrompt(text)
}

func buildPlanClarifyRepairInstruction(state planpkg.State, reply llm.Message, attempt, maxAttempts int) string {
	state = planpkg.NormalizeState(state)
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)

	gaps := "(none recorded)"
	if len(state.DecisionGaps) > 0 {
		items := make([]string, 0, len(state.DecisionGaps))
		for _, gap := range state.DecisionGaps {
			gap = strings.TrimSpace(gap)
			if gap != "" {
				items = append(items, "- "+gap)
			}
		}
		if len(items) > 0 {
			gaps = strings.Join(items, "\n")
		}
	}

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn asked the user to choose among plan options in prose without storing active_choice first.
Attempt %d/%d.

Reply text preview:
%s

Current unresolved decision gaps:
%s

For this next turn:
1) Call update_plan before finalizing.
2) Populate active_choice with a stable id, kind="clarify", one question, and 2 to 4 mutually exclusive options.
3) Put the recommended option first and include explicit shortcuts such as A/B/C. Use one freeform option only if custom input is truly needed.
4) Keep phase in clarify unless this decision fully converges the plan.
5) Keep the visible assistant reply to one short lead sentence only. Do not inline the full A/B/C choice block in prose when the UI can render it.`,
		attempt,
		maxAttempts,
		preview,
		gaps,
	))
}

func shouldRepairPlanDecisionTurn(runMode planpkg.AgentMode, state planpkg.State, intent assistantTurnIntent, reply llm.Message) bool {
	if runMode != planpkg.ModePlan || len(reply.ToolCalls) > 0 {
		return false
	}

	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) || !planpkg.HasDecisionGaps(state) {
		return false
	}

	text := strings.TrimSpace(reply.Content)
	if text == "" || looksLikeClarifyingQuestion(text) {
		return false
	}

	if intent == turnIntentFinalize {
		return true
	}
	return looksLikePlanDecisionAcknowledgement(text)
}

func buildPlanDecisionRepairInstruction(state planpkg.State, reply llm.Message, attempt, maxAttempts int) string {
	state = planpkg.NormalizeState(state)
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)

	gaps := "(none recorded)"
	if len(state.DecisionGaps) > 0 {
		items := make([]string, 0, len(state.DecisionGaps))
		for _, gap := range state.DecisionGaps {
			gap = strings.TrimSpace(gap)
			if gap != "" {
				items = append(items, "- "+gap)
			}
		}
		if len(items) > 0 {
			gaps = strings.Join(items, "\n")
		}
	}

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn appears to have accepted or acted on a user decision in plan mode without calling update_plan first.
Attempt %d/%d.

Reply text preview:
%s

Current unresolved decision gaps:
%s

For this next turn:
1) If the user's latest reply resolves one of these decisions, call update_plan first.
2) Record the chosen option in decision_log and remove or replace the resolved decision_gaps.
3) If no decision gaps remain, populate implementation_brief, set phase to draft or converge_ready as appropriate, and then finalize so the full proposed plan is shown.
4) If another decision is still required, include <turn_intent>ask_user</turn_intent> and ask only the next question.
5) Do not reply with choice-acknowledgement or start-execution text in plan mode without updating plan state first.`,
		attempt,
		maxAttempts,
		preview,
		gaps,
	))
}

func looksLikeClarifyingQuestion(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "question:") ||
		strings.Contains(normalized, "question：") ||
		strings.Contains(normalized, "问题:") ||
		strings.Contains(normalized, "问题：") {
		return true
	}
	hasOptions := strings.Contains(normalized, "a.") && strings.Contains(normalized, "b.")
	if hasOptions && (strings.Contains(normalized, "other:") || strings.Contains(normalized, "other：")) {
		return true
	}
	return false
}

func looksLikeInlineClarifyChoicePrompt(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if looksLikeClarifyingQuestion(text) {
		return true
	}
	if countChoiceShortcuts(normalized) < 2 {
		return false
	}
	if strings.Contains(normalized, "/") || strings.Contains(normalized, "\n") {
		return true
	}
	return containsAnyToken(normalized,
		"choose",
		"pick",
		"select",
		"option",
		"which",
		"prefer",
		"please choose",
		"please pick",
		"please select",
		"\u8bf7\u76f4\u63a5\u9009",
		"\u76f4\u63a5\u9009",
		"\u9009\u4e00\u4e2a",
		"\u9009\u4e00\u4e2a\u65b9\u6848",
		"\u9009\u4e2a\u65b9\u6848",
		"请选择",
		"请先选",
		"先选",
		"选择",
		"选哪个",
		"选目录",
		"选路线",
		"选方案",
	)
}

func looksLikePlanRevisionInput(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if looksLikeExecutionHandoffInput(normalized) || looksLikePlanAdjustmentOnlyInput(normalized) || looksLikeRawPlanChoiceSelection(normalized) {
		return false
	}
	return true
}

func looksLikePlanAdjustmentOnlyInput(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "adjust",
		"adjust plan",
		"continue adjusting plan",
		"continue plan",
		"option 2",
		"option b",
		"2",
		"2.",
		"b",
		"b.",
		"继续微调计划",
		"微调计划",
		"继续调整计划",
		"调整计划":
		return true
	default:
		return false
	}
}

func looksLikeRawPlanChoiceSelection(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "a", "a.", "1", "1.", "c", "c.", "3", "3.", "other":
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "selected_choice:")
}

func countChoiceShortcuts(text string) int {
	matches := clarifyChoiceShortcutPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		seen[strings.ToLower(strings.TrimSpace(match[2]))] = struct{}{}
	}
	return len(seen)
}

func looksLikePlanDecisionAcknowledgement(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return containsAnyToken(normalized,
		"adopt",
		"adopted",
		"go with",
		"going with",
		"chosen",
		"choose ",
		"recorded",
		"using option",
		"start execution",
		"adjust plan",
		"switch to build",
		"采用",
		"已收到",
		"已记录",
		"记录为",
		"选用",
		"选择",
		"开始执行",
		"调整计划",
		"切到 build",
		"切换到 build",
	)
}
