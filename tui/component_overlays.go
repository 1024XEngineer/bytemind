package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderSkillsModal() string {
	lines := []string{modalTitleStyle.Render("Loaded Skills"), mutedStyle.Render("Up/Down to select, Enter to activate, Esc to close"), ""}
	items := m.skillPickerItems()
	if len(items) == 0 {
		lines = append(lines, "No loaded skills available.")
	} else {
		activeName := ""
		if m.sess != nil && m.sess.ActiveSkill != nil {
			activeName = strings.TrimSpace(m.sess.ActiveSkill.Name)
		}
		for i, item := range items {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == clamp(m.commandCursor, 0, len(items)-1) {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}
			label := fmt.Sprintf("%s%s", prefix, item.Name)
			if strings.EqualFold(activeName, item.Name) {
				label += "  (active)"
			}
			lines = append(lines, style.Render(label))
			if strings.TrimSpace(item.Description) != "" {
				lines = append(lines, mutedStyle.Render("   "+item.Description))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func (m model) renderHelpModal() string {
	modalWidth := min(88, max(54, m.width-16))
	innerWidth := max(20, modalWidth-modalBoxStyle.GetHorizontalFrameSize())
	body := renderHelpMarkdown(m.helpText(), innerWidth)
	return modalBoxStyle.Width(modalWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left, modalTitleStyle.Render("Help"), body),
	)
}

func (m model) renderApprovalBanner() string {
	bannerWidth := max(24, m.chatPanelInnerWidth())
	innerWidth := max(20, bannerWidth-approvalBannerStyle.GetHorizontalFrameSize())

	toolName := strings.TrimSpace(m.approval.ToolName)
	if toolName == "" {
		toolName = "unknown"
	}
	command := strings.TrimSpace(m.approval.Command)
	reason := strings.TrimSpace(m.approval.Reason)

	isFullAccessConfirm := strings.EqualFold(strings.TrimSpace(m.approval.Kind), approvalPromptKindEnableFullAccess)

	if isFullAccessConfirm {
		return m.renderFullAccessApproval(innerWidth)
	}

	lines := make([]string, 0, 10)

	// Title line: "Bytemind needs your permission to use {tool}"
	title := fmt.Sprintf("Bytemind needs your permission to use %s", toolName)
	lines = append(lines, approvalTitleStyle.Render(title))

	// Command subtitle
	if command != "" {
		lines = append(lines, approvalReasonStyle.Render(trimToWidth(command, innerWidth)))
	}

	// Reason line if present
	if reason != "" {
		lines = append(lines, approvalReasonStyle.Render(wrapPlainText(reason, innerWidth)))
	}

	lines = append(lines, "") // blank separator

	// Numbered options with ❯ arrow for selected
	options := m.approvalOptions()
	cursor := clamp(m.approval.Cursor, 0, len(options)-1)
	for i, option := range options {
		numStr := fmt.Sprintf("%d", i+1)
		arrow := " "
		style := approvalOptionStyle
		descStyle := approvalOptionDescriptionStyle

		if i == cursor {
			arrow = approvalArrowStyle.Render("❯")
			style = approvalOptionSelectedStyle
			descStyle = approvalOptionDescriptionStyle
		}

		// Format: "  1. Yes" or " ❯ 1. Yes"
		optLine := fmt.Sprintf(" %s %s. %s", arrow, approvalNumberStyle.Render(numStr), style.Render(option.Label))
		lines = append(lines, optLine)

		if option.Description != "" {
			descPrefix := strings.Repeat(" ", 4) // indent under option text
			lines = append(lines, descStyle.Render(descPrefix+wrapPlainText(option.Description, max(8, innerWidth-4))))
		}
	}

	// Feedback input mode
	if m.approval.InFeedback {
		lines = append(lines, "")
		placeholder := "tell Claude what to do next"
		feedbackText := m.approval.Feedback
		if feedbackText == "" {
			lines = append(lines, approvalFeedbackStyle.Render("  "+placeholder))
		} else {
			lines = append(lines, approvalFeedbackStyle.Render("  "+feedbackText))
		}
	}

	// Bottom hint line
	hint := "Esc to cancel"
	if m.approval.InFeedback {
		hint += " · Enter to submit"
	} else {
		hint += " · Tab to amend  · ↑↓ navigate  · Enter select"
	}
	lines = append(lines, "")
	lines = append(lines, approvalHintStyle.Render(hint))

	body := lipgloss.NewStyle().
		Width(innerWidth).
		Render(strings.Join(lines, "\n"))
	return approvalBannerStyle.Render(body)
}

// renderFullAccessApproval renders the simplified enable-full-access confirmation.
func (m model) renderFullAccessApproval(innerWidth int) string {
	lines := make([]string, 0, 6)
	title := "Enable full access?"
	lines = append(lines, approvalTitleStyle.Render(title))

	if reason := strings.TrimSpace(m.approval.Reason); reason != "" {
		lines = append(lines, approvalReasonStyle.Render(trimToWidth(reason, innerWidth)))
	}

	actionText := strings.TrimSpace(m.approval.Command)
	if actionText == "" {
		actionText = "full_access"
	}
	lines = append(lines, approvalCommandStyle.Render("Action: "+trimToWidth(actionText, max(6, innerWidth-8))))

	lines = append(lines, "")

	choice := m.currentApprovalChoice()
	confirmLabel := "Enable"
	rejectLabel := "Cancel"
	confirmTone := "warning"

	confirmChoice := renderApprovalChoice(confirmLabel, confirmTone, choice == approvalChoiceApprove)
	rejectChoice := renderApprovalChoice(rejectLabel, "error", choice == approvalChoiceReject)
	choiceLine := lipgloss.JoinHorizontal(lipgloss.Left, confirmChoice, "  ", rejectChoice)

	hintLine := approvalHintStyle.Render("Use Left/Right to choose, Enter to confirm, Esc to cancel")
	if lipgloss.Width(choiceLine)+2+lipgloss.Width(hintLine) <= innerWidth {
		choiceLine += strings.Repeat(" ", innerWidth-lipgloss.Width(choiceLine)-lipgloss.Width(hintLine)) + hintLine
		hintLine = ""
	}
	lines = append(lines, choiceLine)
	if strings.TrimSpace(hintLine) != "" {
		lines = append(lines, hintLine)
	}

	body := lipgloss.NewStyle().
		Width(innerWidth).
		Render(strings.Join(lines, "\n"))
	return approvalBannerStyle.Render(body)
}

// trimToWidth truncates text to fit within maxWidth chars, adding "…" if needed.
func trimToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	return s[:maxWidth-1] + "…"
}

func renderApprovalChoice(label, tone string, selected bool) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if selected {
		return statusBadgeStyle(tone).Render("> " + label)
	}
	return approvalOptionIdleStyle.Render("  " + label)
}

func (m model) renderActiveSkillBanner() string {
	if m.sess == nil || m.sess.ActiveSkill == nil {
		return ""
	}
	name := strings.TrimSpace(m.sess.ActiveSkill.Name)
	if name == "" {
		return ""
	}

	line := "Active skill: " + name
	if len(m.sess.ActiveSkill.Args) > 0 {
		keys := make([]string, 0, len(m.sess.ActiveSkill.Args))
		for key := range m.sess.ActiveSkill.Args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, m.sess.ActiveSkill.Args[key]))
		}
		line += " | args: " + strings.Join(pairs, ", ")
	}

	width := max(24, m.chatPanelInnerWidth())
	return activeSkillBannerStyle.Width(width).Render(accentStyle.Render(line))
}
