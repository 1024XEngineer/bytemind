package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func RenderEnhancedStatusBar(sections []string) string {
	return statusBarStyle.Render(lipgloss.JoinHorizontal(lipgloss.Left, sections...))
}

func RenderConnectionStatus(connected bool, details string) string {
	label := "Disconnected"
	style := errorStyle
	if connected {
		label = "Connected"
		style = doneStyle
	}
	if details != "" {
		label += " (" + details + ")"
	}
	return style.Render(label)
}

func RenderTokenUsage(current, max int, _ string) string {
	if max <= 0 {
		return mutedStyle.Render(fmt.Sprintf("%d tokens", current))
	}
	return accentStyle.Render(fmt.Sprintf("%d/%d tokens", current, max))
}

func RenderCurrentTime() string {
	return mutedStyle.Render(time.Now().Format("15:04:05"))
}

func RenderStatus(status string, statusType string) string {
	switch statusType {
	case "active", "running", "success":
		return doneStyle.Render(status)
	case "warning", "pending":
		return warnStyle.Render(status)
	case "error", "failed":
		return errorStyle.Render(status)
	default:
		return mutedStyle.Render(status)
	}
}

func RenderProgressBar(current, total int, label string) string {
	if total <= 0 {
		return mutedStyle.Render(label)
	}
	return accentStyle.Render(fmt.Sprintf("%s %d/%d", label, current, total))
}

func RenderKeyHint(key, description string) string {
	return lipgloss.JoinHorizontal(lipgloss.Left, footerHintKeyStyle.Render(key), footerHintLabelStyle.Render(" "+description))
}

func RenderKeyHints(hints []string) string {
	rendered := make([]string, 0, len(hints))
	for _, hint := range hints {
		rendered = append(rendered, footerHintKeyStyle.Render(hint))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, rendered...)
}

func RenderAnimatedStatus(status string, _ int) string {
	return doneStyle.Render(status)
}
