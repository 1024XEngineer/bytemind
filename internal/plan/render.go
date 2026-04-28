package plan

import (
	"fmt"
	"strings"
)

func RenderStructuredPlanBlock(state State) string {
	state = NormalizeState(state)
	if !ShouldRenderStructuredPlanBlock(state) {
		return ""
	}

	lines := []string{"<proposed_plan>"}
	if goal := strings.TrimSpace(state.Goal); goal != "" {
		lines = appendSection(lines, "Goal", []string{"- " + goal})
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		lines = appendSection(lines, "Summary", []string{"- " + summary})
	}
	if brief := strings.TrimSpace(state.ImplementationBrief); brief != "" {
		lines = appendSection(lines, "Implementation Brief", renderImplementationBriefLines(brief))
	}
	if phase := NormalizePhase(string(state.Phase)); phase != PhaseNone {
		lines = appendSection(lines, "Phase", []string{"- " + string(phase)})
	}
	if len(state.DecisionLog) > 0 {
		items := make([]string, 0, len(state.DecisionLog))
		for _, entry := range state.DecisionLog {
			line := "- " + entry.Decision
			if entry.Reason != "" {
				line += " (Why: " + entry.Reason + ")"
			}
			items = append(items, line)
		}
		lines = appendSection(lines, "Decision Log", items)
	}
	if len(state.DecisionGaps) > 0 {
		items := make([]string, 0, len(state.DecisionGaps))
		for _, gap := range state.DecisionGaps {
			items = append(items, "- "+gap)
		}
		lines = appendSection(lines, "Decision Gaps", items)
	}
	if len(state.Steps) > 0 {
		items := make([]string, 0, len(state.Steps))
		for i, step := range state.Steps {
			line := fmt.Sprintf("%d. [%s] %s", i+1, step.Status, step.Title)
			if step.Description != "" {
				line += " - " + step.Description
			}
			items = append(items, line)
		}
		lines = appendSection(lines, "Plan", items)
	}
	if len(state.Risks) > 0 {
		items := make([]string, 0, len(state.Risks))
		for _, risk := range state.Risks {
			items = append(items, "- "+risk)
		}
		lines = appendSection(lines, "Risks", items)
	}
	if len(state.Verification) > 0 {
		items := make([]string, 0, len(state.Verification))
		for _, item := range state.Verification {
			items = append(items, "- "+item)
		}
		lines = appendSection(lines, "Verification", items)
	}
	readiness := []string{
		checklistLine(state.ScopeDefined, "Scope defined"),
		checklistLine(state.RiskRollbackDefined, "Risks and rollback defined"),
		checklistLine(state.VerificationDefined, "Verification path defined"),
	}
	lines = appendSection(lines, "Execution Readiness", readiness)
	if next := strings.TrimSpace(state.NextAction); next != "" {
		lines = appendSection(lines, "Next Action", []string{"- " + next})
	}
	if reason := strings.TrimSpace(state.BlockReason); reason != "" {
		lines = appendSection(lines, "Blocked Reason", []string{"- " + reason})
	}
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	lines = append(lines, "</proposed_plan>")
	return strings.Join(lines, "\n")
}

func RenderPromptStateBlock(state State) string {
	state = NormalizeState(state)
	if !hasRenderablePlanState(state) {
		return ""
	}

	lines := []string{}
	if phase := NormalizePhase(string(state.Phase)); phase != PhaseNone {
		lines = append(lines, "phase: "+string(phase))
	}
	if goal := strings.TrimSpace(state.Goal); goal != "" {
		lines = append(lines, "goal: "+goal)
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		lines = append(lines, "summary: "+summary)
	}
	if len(state.DecisionGaps) > 0 {
		lines = append(lines, "decision_gaps: "+strings.Join(state.DecisionGaps, " | "))
	}
	if state.ActiveChoice != nil {
		lines = append(lines, "active_choice:")
		if state.ActiveChoice.ID != "" {
			lines = append(lines, "- id: "+state.ActiveChoice.ID)
		}
		if state.ActiveChoice.Kind != "" {
			lines = append(lines, "- kind: "+state.ActiveChoice.Kind)
		}
		lines = append(lines, "- question: "+state.ActiveChoice.Question)
		for _, option := range state.ActiveChoice.Options {
			label := fmt.Sprintf("- [%s] %s", option.Shortcut, option.Title)
			if option.Description != "" {
				label += " -- " + option.Description
			}
			if option.Recommended {
				label += " -- recommended"
			}
			if option.Freeform {
				label += " -- freeform"
			}
			lines = append(lines, label)
		}
	}
	if len(state.DecisionLog) > 0 {
		lines = append(lines, "decision_log:")
		for _, entry := range state.DecisionLog {
			line := "- " + entry.Decision
			if entry.Reason != "" {
				line += " -- " + entry.Reason
			}
			lines = append(lines, line)
		}
	}
	if len(state.Steps) > 0 {
		lines = append(lines, "steps:")
		for _, step := range state.Steps {
			lines = append(lines, fmt.Sprintf("- [%s] %s", step.Status, step.Title))
		}
	}
	if len(state.Risks) > 0 {
		lines = append(lines, "risks: "+strings.Join(state.Risks, " | "))
	}
	if len(state.Verification) > 0 {
		lines = append(lines, "verification: "+strings.Join(state.Verification, " | "))
	}
	if state.ScopeDefined || state.RiskRollbackDefined || state.VerificationDefined {
		lines = append(lines, fmt.Sprintf(
			"execution_readiness: scope=%s, risks_rollback=%s, verification=%s",
			yesNo(state.ScopeDefined),
			yesNo(state.RiskRollbackDefined),
			yesNo(state.VerificationDefined),
		))
	}
	if next := strings.TrimSpace(state.NextAction); next != "" {
		lines = append(lines, "next_action: "+next)
	}
	if reason := strings.TrimSpace(state.BlockReason); reason != "" {
		lines = append(lines, "block_reason: "+reason)
	}
	return strings.Join(lines, "\n")
}

func ShouldRenderStructuredPlanBlock(state State) bool {
	state = NormalizeState(state)
	if !hasRenderablePlanState(state) || !HasStructuredPlan(state) || HasDecisionGaps(state) {
		return false
	}
	switch NormalizePhase(string(state.Phase)) {
	case PhaseExplore, PhaseClarify:
		return false
	default:
		return true
	}
}

func hasRenderablePlanState(state State) bool {
	return HasStructuredPlan(state) ||
		strings.TrimSpace(state.Goal) != "" ||
		strings.TrimSpace(state.Summary) != "" ||
		strings.TrimSpace(state.ImplementationBrief) != "" ||
		len(state.DecisionLog) > 0 ||
		len(state.DecisionGaps) > 0 ||
		state.ActiveChoice != nil ||
		len(state.Risks) > 0 ||
		len(state.Verification) > 0
}

func appendSection(lines []string, title string, items []string) []string {
	if len(items) == 0 {
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	lines = append(lines, "## "+title)
	lines = append(lines, "")
	lines = append(lines, items...)
	return lines
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func splitDocumentLines(text string) []string {
	rawLines := strings.Split(strings.TrimSpace(text), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func renderImplementationBriefLines(text string) []string {
	rawLines := splitDocumentLines(text)
	lines := make([]string, 0, len(rawLines)*2)
	for _, raw := range rawLines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if label, rest, ok := splitDocumentLabel(line); ok {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, "### "+label)
			if rest != "" {
				lines = append(lines, rest)
			}
			continue
		}
		if len(lines) == 0 {
			lines = append(lines, line)
			continue
		}
		last := lines[len(lines)-1]
		switch {
		case last == "":
			lines = append(lines, line)
		case strings.HasPrefix(last, "### "), strings.HasPrefix(last, "## "), strings.HasPrefix(last, "- "), isOrderedMarkdownLine(last):
			lines = append(lines, line)
		default:
			lines[len(lines)-1] = strings.TrimSpace(last + " " + line)
		}
	}
	return lines
}

func splitDocumentLabel(line string) (label string, rest string, ok bool) {
	separator := ":"
	index := strings.Index(line, separator)
	if index < 0 {
		separator = "："
		index = strings.Index(line, separator)
	}
	if index <= 0 {
		return "", "", false
	}
	label = strings.TrimSpace(line[:index])
	rest = strings.TrimSpace(line[index+len(separator):])
	if label == "" || len([]rune(label)) > 32 {
		return "", "", false
	}
	return label, rest, true
}

func checklistLine(done bool, label string) string {
	if done {
		return "- [x] " + label
	}
	return "- [ ] " + label
}

func isOrderedMarkdownLine(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) < 3 {
		return false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	return index > 0 && len(line) > index+1 && line[index] == '.' && line[index+1] == ' '
}
