package tui

import (
	"fmt"
	"strings"

	planpkg "bytemind/internal/plan"
)

func parsePlanSteps(raw string) []string {
	parts := strings.Split(raw, "|")
	steps := make([]string, 0, len(parts))
	for _, part := range parts {
		step := strings.TrimSpace(part)
		if step != "" {
			steps = append(steps, step)
		}
	}
	return steps
}

func canContinuePlan(state planpkg.State) bool {
	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return false
	}
	switch planpkg.NormalizePhase(string(state.Phase)) {
	case planpkg.PhaseBlocked, planpkg.PhaseCompleted:
		return false
	default:
		return true
	}
}

func currentOrNextStepTitle(state planpkg.State) string {
	state = planpkg.NormalizeState(state)
	if step, ok := planpkg.CurrentStep(state); ok && strings.TrimSpace(step.Title) != "" {
		return strings.TrimSpace(step.Title)
	}
	for _, step := range state.Steps {
		if planpkg.NormalizeStepStatus(string(step.Status)) == planpkg.StepPending && strings.TrimSpace(step.Title) != "" {
			return strings.TrimSpace(step.Title)
		}
	}
	return ""
}

func preparePlanForContinuation(state planpkg.State) (planpkg.State, error) {
	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return state, fmt.Errorf("no structured plan is available to continue")
	}
	switch planpkg.NormalizePhase(string(state.Phase)) {
	case planpkg.PhaseBlocked:
		if state.BlockReason != "" {
			return state, fmt.Errorf("plan is blocked: %s", state.BlockReason)
		}
		return state, fmt.Errorf("plan is blocked and cannot continue yet")
	case planpkg.PhaseCompleted:
		return state, fmt.Errorf("plan is already completed")
	}

	if _, ok := planpkg.CurrentStep(state); !ok {
		for i := range state.Steps {
			if planpkg.NormalizeStepStatus(string(state.Steps[i].Status)) == planpkg.StepPending {
				state.Steps[i].Status = planpkg.StepInProgress
				break
			}
		}
	}

	state.Phase = planpkg.PhaseExecuting
	if strings.TrimSpace(state.NextAction) == "" {
		state.NextAction = planpkg.DefaultNextAction(state)
	}
	return planpkg.NormalizeState(state), nil
}

func copyPlanState(state planpkg.State) planpkg.State {
	return planpkg.CloneState(state)
}

func toAgentMode(mode planpkg.AgentMode) agentMode {
	if planpkg.NormalizeMode(string(mode)) == planpkg.ModePlan {
		return modePlan
	}
	return modeBuild
}

func (m model) hasPlanPanel() bool {
	return false
}

func (m model) showPlanSidebar() bool {
	return m.hasPlanPanel() && m.chatPanelInnerWidth() >= 104
}

func (m model) planPanelWidth() int {
	if !m.showPlanSidebar() {
		return m.chatPanelInnerWidth()
	}
	return clamp(m.chatPanelInnerWidth()/3, 30, 42)
}

func (m model) conversationPanelWidth() int {
	width := m.chatPanelInnerWidth()
	if m.showPlanSidebar() {
		width -= m.planPanelWidth() + 1
	}
	return max(24, width)
}

func statusGlyph(status string) string {
	switch planpkg.NormalizeStepStatus(status) {
	case planpkg.StepCompleted:
		return doneStyle.Render("v")
	case planpkg.StepInProgress:
		return accentStyle.Render(">")
	case planpkg.StepBlocked:
		return errorStyle.Render("!")
	default:
		switch status {
		case "warn":
			return warnStyle.Render("!")
		case "error":
			return errorStyle.Render("x")
		default:
			return mutedStyle.Render("-")
		}
	}
}
