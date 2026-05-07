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

func (m model) renderModelsModal() string {
	title := "Models"
	hint := "Up/Down to select, Enter to switch, Esc to close"
	if normalizeModelPickerMode(m.modelPickerMode) == modelPickerModeDelete {
		title = "Delete Model"
		hint = "Up/Down to select, Enter to delete, Esc to close"
	}
	lines := []string{
		modalTitleStyle.Render(title),
		mutedStyle.Render(hint),
		"",
		"Current: " + activeModelLabel(m.cfg),
		"",
	}
	targets := m.modelPickerTargets()
	if len(targets) == 0 {
		if normalizeModelPickerMode(m.modelPickerMode) == modelPickerModeDelete {
			lines = append(lines, "No configured models available to delete.")
		} else {
			lines = append(lines, "No switchable models available.")
		}
	} else {
		activeProvider, activeModel := activeProviderAndModel(m.cfg)
		defaultProvider := strings.TrimSpace(m.cfg.ProviderRuntime.DefaultProvider)
		defaultModel := strings.TrimSpace(m.cfg.ProviderRuntime.DefaultModel)
		for i, target := range targets {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == clamp(m.commandCursor, 0, len(targets)-1) {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}

			label := prefix + modelTargetLabel(target)
			flags := make([]string, 0, 2)
			if strings.EqualFold(strings.TrimSpace(string(target.ProviderID)), activeProvider) &&
				strings.TrimSpace(string(target.ModelID)) == activeModel {
				flags = append(flags, "active")
			}
			if strings.EqualFold(strings.TrimSpace(string(target.ProviderID)), defaultProvider) &&
				strings.TrimSpace(string(target.ModelID)) == defaultModel {
				flags = append(flags, "default")
			}
			if len(flags) > 0 {
				label += "  (" + strings.Join(flags, ", ") + ")"
			}
			lines = append(lines, style.Render(label))

			metadata := target.ModelMetadata()
			details := make([]string, 0, 3)
			if metadata.Family != "" {
				details = append(details, "family="+metadata.Family)
			}
			if metadata.ContextWindow > 0 {
				details = append(details, fmt.Sprintf("context=%d", metadata.ContextWindow))
			}
			if metadata.UsageSource != "" {
				details = append(details, "source="+metadata.UsageSource)
			}
			if len(details) > 0 {
				lines = append(lines, mutedStyle.Render("   "+strings.Join(details, "  ")))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(104, max(60, m.width-12))).Render(strings.Join(lines, "\n"))
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
	if command == "" {
		command = "-"
	}

	isFullAccessConfirm := strings.EqualFold(strings.TrimSpace(m.approval.Kind), approvalPromptKindEnableFullAccess)
	title := "Approval required"
	lines := []string{}
	if isFullAccessConfirm {
		toolPrefix := "Action: "
		confirmLabel := "Enable"
		rejectLabel := "Cancel"
		confirmTone := "warning"
		title = "Enable full access?"

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
		lines = []string{line1, actionLine, choiceLine}
		if strings.TrimSpace(hintLine) != "" {
			lines = append(lines, hintLine)
		}
	} else {
		lines = []string{
			approvalTitleStyle.Render(title),
			approvalReasonStyle.Render("Tool: " + trimPreview(toolName, innerWidth-6)),
			approvalCommandStyle.Render("Command: " + trimPreview(command, innerWidth-9)),
		}
		if reason := strings.TrimSpace(m.approval.Reason); reason != "" {
			lines = append(lines, approvalReasonStyle.Render(wrapPlainText(reason, innerWidth)))
		}
		lines = append(lines, "")
		for i, option := range m.approvalOptions() {
			prefix := "  "
			style := approvalOptionStyle
			if i == clamp(m.approval.Cursor, 0, len(m.approvalOptions())-1) {
				prefix = "> "
				style = approvalOptionSelectedStyle
			}
			lines = append(lines, style.Render(prefix+option.Label))
			lines = append(lines, approvalOptionDescriptionStyle.Render("  "+wrapPlainText(option.Description, max(8, innerWidth-2))))
		}
		lines = append(lines, "", approvalHintStyle.Render("Up/Down or J/K to select  Enter confirm  Y approve once  N/Esc reject"))
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
