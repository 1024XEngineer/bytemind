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
	command := strings.TrimSpace(m.approval.Command)
	if command == "" {
		command = "-"
	}

	isFullAccessConfirm := strings.EqualFold(strings.TrimSpace(m.approval.Kind), approvalPromptKindEnableFullAccess)
	title := "Approval required"
	toolPrefix := "Tool: "
	confirmLabel := "Approve"
	rejectLabel := "Reject"
	confirmTone := "success"
	if isFullAccessConfirm {
		title = "Enable full access?"
		toolPrefix = "Action: "
		confirmLabel = "Enable"
		rejectLabel = "Cancel"
		confirmTone = "warning"
	}

	reasonBudget := max(0, innerWidth-lipgloss.Width(title)-2)
	reason := trimPreview(m.approval.Reason, reasonBudget)
	line1 := approvalTitleStyle.Render(title)
	if reason != "" {
		line1 += "  " + approvalReasonStyle.Render(reason)
	}

	actionLine := approvalCommandStyle.Render(toolPrefix + trimPreview(command, max(6, innerWidth-lipgloss.Width(toolPrefix))))

	choice := m.currentApprovalChoice()
	confirmChoice := renderApprovalChoice(confirmLabel, confirmTone, choice == approvalChoiceApprove)
	rejectChoice := renderApprovalChoice(rejectLabel, "error", choice == approvalChoiceReject)
	choiceLine := lipgloss.JoinHorizontal(lipgloss.Left, confirmChoice, "  ", rejectChoice)

	hintLine := approvalHintStyle.Render("Use Left/Right to choose, Enter to confirm, Esc to cancel")
	if lipgloss.Width(choiceLine)+2+lipgloss.Width(hintLine) <= innerWidth {
		choiceLine += strings.Repeat(" ", innerWidth-lipgloss.Width(choiceLine)-lipgloss.Width(hintLine)) + hintLine
		hintLine = ""
	}
	lines := []string{line1, actionLine, choiceLine}
	if strings.TrimSpace(hintLine) != "" {
		lines = append(lines, hintLine)
	}

	body := lipgloss.NewStyle().
		Width(innerWidth).
		Render(strings.Join(lines, "\n"))
	return approvalBannerStyle.Render(body)
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
