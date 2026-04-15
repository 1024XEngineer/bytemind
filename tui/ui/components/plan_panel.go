package tui

import (
	"fmt"
	"strings"

	planpkg "bytemind/internal/plan"
)

func (m model) planModeLabel() string {
	if m.mode == modePlan {
		return "PLAN"
	}
	return "BUILD"
}

func (m model) planPhaseLabel() string {
	phase := planpkg.NormalizePhase(string(m.plan.Phase))
	if phase == planpkg.PhaseNone && m.mode == modePlan {
		phase = planpkg.PhaseDrafting
	}
	if phase == planpkg.PhaseNone {
		return "none"
	}
	return string(phase)
}

func (m model) renderPlanPanel(width int) string {
	width = max(24, width)
	return modalBoxStyle.Width(width).Render(m.planView.View())
}

func (m model) planPanelContent(width int) string {
	width = max(16, width)
	lines := []string{
		accentStyle.Render(m.planModeLabel()),
		mutedStyle.Render("Phase: " + m.planPhaseLabel()),
	}

	if goal := strings.TrimSpace(m.plan.Goal); goal != "" {
		lines = append(lines, "", cardTitleStyle.Render("Goal"), wrapPlainText(goal, width))
	}
	if summary := strings.TrimSpace(m.plan.Summary); summary != "" {
		lines = append(lines, "", cardTitleStyle.Render("Summary"), wrapPlainText(summary, width))
	}

	lines = append(lines, "", cardTitleStyle.Render("Steps"))
	if len(m.plan.Steps) == 0 {
		lines = append(lines, mutedStyle.Render("No structured plan yet. Use update_plan to create one."))
	} else {
		for _, step := range m.plan.Steps {
			lines = append(lines, m.renderPlanStep(step, width), "")
		}
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	if nextAction := strings.TrimSpace(m.plan.NextAction); nextAction != "" {
		lines = append(lines, "", cardTitleStyle.Render("Next Action"), wrapPlainText(nextAction, width))
	}
	if reason := strings.TrimSpace(m.plan.BlockReason); reason != "" {
		lines = append(lines, "", cardTitleStyle.Render("Blocked Reason"), errorStyle.Render(wrapPlainText(reason, width)))
	}

	return strings.Join(lines, "\n")
}

func (m model) planPanelRenderHeight() int {
	if !m.hasPlanPanel() {
		return 0
	}
	return m.planView.Height + modalBoxStyle.GetVerticalFrameSize()
}

func (m model) renderPlanStep(step planpkg.Step, width int) string {
	header := fmt.Sprintf("%s %s", statusGlyph(string(step.Status)), step.Title)
	parts := []string{wrapPlainText(header, width)}
	if desc := strings.TrimSpace(step.Description); desc != "" {
		parts = append(parts, mutedStyle.Render(wrapPlainText(desc, width)))
	}
	if len(step.Files) > 0 {
		parts = append(parts, mutedStyle.Render("Files: "+compact(strings.Join(step.Files, ", "), width)))
	}
	if len(step.Verify) > 0 {
		parts = append(parts, mutedStyle.Render("Verify: "+compact(strings.Join(step.Verify, " | "), width)))
	}
	if risk := strings.TrimSpace(string(step.Risk)); risk != "" {
		parts = append(parts, mutedStyle.Render("Risk: "+risk))
	}
	return strings.Join(parts, "\n")
}
