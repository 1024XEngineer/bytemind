package plan

import "strings"

const StructuredPlanReminder = "Plan mode requires a structured plan before finishing. Please restate the plan using update_plan."

// FinalizeAssistantAnswer enforces plan-mode completion policy on final text.
func FinalizeAssistantAnswer(mode AgentMode, state State, answer string) string {
	if mode != ModePlan || HasStructuredPlan(state) {
		return answer
	}
	clean := strings.TrimSpace(answer)
	if clean == "" {
		return StructuredPlanReminder
	}
	return clean + "\n\n" + StructuredPlanReminder
}
