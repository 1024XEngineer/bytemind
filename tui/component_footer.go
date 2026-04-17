package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

func (m model) renderFooter() string {
	ensureZoneManager()
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(zone.Mark(inputEditorZoneID, m.renderInputEditorView()))
	parts := make([]string, 0, 4)
	if m.approval != nil {
		parts = append(parts, m.renderApprovalBanner())
	}
	if m.startupGuide.Active {
		parts = append(parts, m.renderStartupGuidePanel())
	} else if m.promptSearchOpen {
		parts = append(parts, m.renderPromptSearchPalette())
	} else if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette())
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette())
	}
	if banner := m.renderActiveSkillBanner(); banner != "" {
		parts = append(parts, banner)
	}
	parts = append(parts, inputBorder, m.renderFooterInfoLine())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderRunIndicator() string {
	if !m.busy {
		return ""
	}
	width := max(24, m.chatPanelInnerWidth())
	return runIndicatorStyle.Width(width).Render(m.runIndicatorText())
}

func runIndicatorPhaseText(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "thinking":
		return "thinking..."
	case "responding":
		return "Responding..."
	case "tool":
		return "Running tool..."
	case "interrupting":
		return "Interrupting..."
	case "approval":
		return "Waiting for approval..."
	default:
		return "Working..."
	}
}

func formatElapsedClock(startedAt, now time.Time) string {
	if startedAt.IsZero() || now.Before(startedAt) {
		return "00:00"
	}
	seconds := int(now.Sub(startedAt).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func (m model) runIndicatorText() string {
	spin := strings.TrimSpace(m.spinner.View())
	if spin == "" {
		spin = "⠋"
	}
	indicator := fmt.Sprintf("%s %s", spin, runIndicatorPhaseText(m.phase))
	if strings.EqualFold(strings.TrimSpace(m.phase), "thinking") {
		indicator = thinkingIndicatorStyle.Render(indicator)
	}
	return fmt.Sprintf("%s (%s)", indicator, formatElapsedClock(m.runStartedAt, time.Now()))
}

func (m model) renderModeTabs() string {
	buildStyle := modeTabStyle.Copy().Foreground(colorMuted)
	planStyle := modeTabStyle.Copy().Foreground(colorMuted)
	if m.mode == modeBuild {
		buildStyle = buildStyle.Copy().Foreground(colorAccent).Bold(true)
	} else {
		planStyle = planStyle.Copy().Foreground(colorThinking).Bold(true)
	}
	parts := []string{
		buildStyle.Render("Build"),
		planStyle.Render("Plan"),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m model) renderFooterInfoLine() string {
	width := max(24, m.chatPanelInnerWidth())
	left := m.renderModeTabs()
	modelName := strings.TrimSpace(m.currentModelLabel())
	if modelName == "-" {
		modelName = ""
	}
	right := renderFooterInfoRight(modelName, 1<<30)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 2 {
		available := max(10, width-leftW-2)
		if available <= 10 {
			return lipgloss.NewStyle().Width(width).Render(renderFooterInfoRight(modelName, width))
		}
		compacted := renderFooterInfoRight(modelName, available)
		gap = width - leftW - lipgloss.Width(compacted)
		return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", max(2, gap)) + compacted)
	}

	return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

func renderFooterInfoRight(modelName string, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return renderInlineShortcutHintsCompacted(footerShortcutHints, maxWidth)
	}
	modelText := compact(modelName, maxWidth)
	modelWidth := runewidth.StringWidth(modelText)
	if modelWidth >= maxWidth {
		return mutedStyle.Render(modelText)
	}
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)
	remaining := maxWidth - modelWidth - dividerWidth
	if remaining <= 0 {
		return mutedStyle.Render(modelText)
	}
	hints := renderInlineShortcutHintsCompacted(footerShortcutHints, remaining)
	if strings.TrimSpace(hints) == "" {
		return mutedStyle.Render(modelText)
	}
	return mutedStyle.Render(modelText) + footerHintDividerStyle.Render(dividerPlain) + hints
}

func renderFooterShortcutHints() string {
	return renderInlineShortcutHints(footerShortcutHints)
}

func renderInlineShortcutHints(hints []footerShortcutHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, footerHintKeyStyle.Render(hint.Key)+" "+footerHintLabelStyle.Render(hint.Label))
	}
	return strings.Join(parts, footerHintDividerStyle.Render("  |  "))
}

func renderInlineShortcutHintsCompacted(hints []footerShortcutHint, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)

	used := 0
	parts := make([]string, 0, len(hints)*2)
	for _, hint := range hints {
		key := strings.TrimSpace(hint.Key)
		label := strings.TrimSpace(hint.Label)
		if key == "" {
			continue
		}
		segmentPlain := key
		segmentStyled := footerHintKeyStyle.Render(key)
		if label != "" {
			segmentPlain += " " + label
			segmentStyled += " " + footerHintLabelStyle.Render(label)
		}
		needDivider := len(parts) > 0
		prefixWidth := 0
		if needDivider {
			prefixWidth = dividerWidth
		}
		segmentWidth := runewidth.StringWidth(segmentPlain)
		if used+prefixWidth+segmentWidth <= maxWidth {
			if needDivider {
				parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
				used += dividerWidth
			}
			parts = append(parts, segmentStyled)
			used += segmentWidth
			continue
		}

		remaining := maxWidth - used - prefixWidth
		if remaining <= 0 {
			break
		}
		if needDivider {
			parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
			used += dividerWidth
		}

		keyWidth := runewidth.StringWidth(key)
		if keyWidth >= remaining {
			parts = append(parts, footerHintKeyStyle.Render(compact(key, remaining)))
			break
		}
		if label == "" {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		labelSpace := remaining - keyWidth - 1
		if labelSpace <= 0 {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		parts = append(parts, footerHintKeyStyle.Render(key)+" "+footerHintLabelStyle.Render(compact(label, labelSpace)))
		break
	}
	return strings.Join(parts, "")
}
