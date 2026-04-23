package plan

import (
	"regexp"
	"strings"
)

const StructuredPlanReminder = "Plan mode requires a structured plan before finishing. Please restate the plan using update_plan."

var proposedPlanBlockPattern = regexp.MustCompile(`(?is)<proposed_plan>.*?</proposed_plan>`)

// FinalizeAssistantAnswer enforces plan-mode completion policy on final text.
func FinalizeAssistantAnswer(mode AgentMode, state State, answer string) string {
	if mode != ModePlan {
		return answer
	}
	clean := strings.TrimSpace(answer)
	if !HasStructuredPlan(state) {
		if clean == "" {
			return StructuredPlanReminder
		}
		return clean + "\n\n" + StructuredPlanReminder
	}
	clean = strings.TrimSpace(proposedPlanBlockPattern.ReplaceAllString(clean, ""))
	block := RenderStructuredPlanBlock(state)
	if block == "" {
		return clean
	}
	mainBody, actionTail := splitPlanActionTail(clean)
	if clean == "" {
		return block
	}
	if actionTail == "" {
		return clean + "\n\n" + block
	}
	if strings.TrimSpace(mainBody) == "" {
		return block + "\n\n" + actionTail
	}
	return strings.TrimSpace(mainBody) + "\n\n" + block + "\n\n" + actionTail
}

func splitPlanActionTail(text string) (string, string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return "", ""
	}

	if index := findActionTailStart(lines); index >= 0 {
		head := strings.TrimSpace(strings.Join(lines[:index], "\n"))
		tail := strings.TrimSpace(strings.Join(lines[index:], "\n"))
		return head, tail
	}
	return strings.TrimSpace(text), ""
}

func findActionTailStart(lines []string) int {
	for i, line := range lines {
		if isActionHeading(line) {
			return i
		}
	}

	firstOption := -1
	actionLines := 0
	for i, line := range lines {
		if isActionOption(line) || isExecutionHint(line) {
			actionLines++
			if firstOption < 0 {
				firstOption = i
			}
		}
	}
	if firstOption >= 0 && actionLines >= 2 {
		return firstOption
	}
	return -1
}

func isActionHeading(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(normalized, "请选择下一步") ||
		strings.HasPrefix(normalized, "可选下一步") ||
		strings.HasPrefix(normalized, "choose next step") ||
		strings.HasPrefix(normalized, "choose next action") ||
		strings.HasPrefix(normalized, "next step") ||
		strings.HasPrefix(normalized, "next action")
}

func isActionOption(line string) bool {
	normalized := normalizeActionLine(line)
	return normalized == "start execution" ||
		normalized == "adjust plan" ||
		normalized == "开始执行" ||
		normalized == "调整计划"
}

func isExecutionHint(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	return (strings.Contains(normalized, "start execution") || strings.Contains(normalized, "continue execution")) &&
		strings.Contains(normalized, "build") ||
		(strings.Contains(normalized, "开始执行") || strings.Contains(normalized, "继续执行")) &&
			(strings.Contains(normalized, "build") || strings.Contains(normalized, "切换"))
}

func normalizeActionLine(line string) string {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	trimmed = strings.TrimPrefix(trimmed, "-")
	trimmed = strings.TrimPrefix(trimmed, "*")
	trimmed = strings.TrimSpace(trimmed)
	for _, prefix := range []string{"1.", "2.", "a.", "b.", "1)", "2)", "a)", "b)"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			break
		}
	}
	return strings.TrimSpace(trimmed)
}
