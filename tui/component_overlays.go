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
	width := max(24, m.chatPanelInnerWidth())
	commandWidth := max(20, width-4)
	lines := []string{
		accentStyle.Render("Approval required"),
		mutedStyle.Render("Reason: " + trimPreview(m.approval.Reason, commandWidth)),
		codeStyle.Width(commandWidth).Render(m.approval.Command),
		mutedStyle.Render("Y / Enter approve    N / Esc reject"),
	}
	return approvalBannerStyle.Width(width).Render(strings.Join(lines, "\n"))
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
