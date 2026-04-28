package agent

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func shouldRepairPlanBootstrapTurn(runMode planpkg.AgentMode, state planpkg.State, reply llm.Message) bool {
	if runMode != planpkg.ModePlan || len(reply.ToolCalls) > 0 {
		return false
	}
	state = planpkg.NormalizeState(state)
	if planpkg.HasStructuredPlan(state) {
		return false
	}
	return strings.TrimSpace(reply.Content) != "" || strings.TrimSpace(reply.Text()) != ""
}

func buildPlanBootstrapRepairInstruction(reply llm.Message, latestUser string, attempt, maxAttempts int, messages []llm.Message, availableTools []string) string {
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)

	latestUser = strings.TrimSpace(latestUser)
	if latestUser == "" {
		latestUser = "(empty user input)"
	}

	observedActivity := "(none yet)"
	if hasToolActivitySinceLatestHumanUser(messages) {
		observedActivity = "read-only tool results already exist in this planning cycle"
	}

	recommendedTools := make([]string, 0, 5)
	for _, toolName := range []string{"list_files", "search_text", "read_file", "web_search", "web_fetch"} {
		if containsToolName(availableTools, toolName) {
			recommendedTools = append(recommendedTools, toolName)
		}
	}
	toolHint := "(none)"
	if len(recommendedTools) > 0 {
		toolHint = strings.Join(recommendedTools, ", ")
	}

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn stayed in plan mode without creating the initial structured plan state.
Attempt %d/%d.

Latest user planning request:
%s

Reply text preview:
%s

Observed tool activity in this planning cycle:
%s

Recommended read-only investigation tools for this turn:
%s

For this next turn:
1) Start working immediately. Do not ask the user for permission to inspect the repo or gather evidence.
2) If you do not yet have enough evidence, emit focused read-only tool calls now. Prefer local inspection first, and use web_search/web_fetch only when current or external evidence is actually needed.
3) After enough evidence is gathered, call update_plan before finalizing. Create a real plan skeleton with 3 to 7 ordered steps, summary, phase, decision_gaps, and verification/risks when known.
4) If a key decision is still open, store it in active_choice inside update_plan and then ask only that one focused question with <turn_intent>ask_user</turn_intent>.
5) If the plan is ready to show, finalize with a short acknowledgement only after update_plan succeeds. Do not end a plan-mode turn with plain prose before the structured plan exists.`,
		attempt,
		maxAttempts,
		latestUser,
		preview,
		observedActivity,
		toolHint,
	))
}
