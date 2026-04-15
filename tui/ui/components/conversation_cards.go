package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderChatCopySection(item chatEntry, width int) string {
	title := strings.TrimSpace(item.Title)
	status := strings.TrimSpace(item.Status)
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "assistant":
		if strings.EqualFold(item.Status, "thinking") || strings.EqualFold(item.Status, "thinking_done") {
			title = ""
			status = ""
		}
	}

	if title == "" {
		switch item.Kind {
		case "assistant":
			title = assistantLabel
		case "user":
			title = "You"
		case "tool":
			title = "Tool"
		default:
			title = "Message"
		}
	}
	if status != "" {
		title += "  " + status
	}

	body := strings.TrimRight(formatChatBody(item, width), "\n")
	if strings.TrimSpace(body) == "" {
		return title
	}
	return title + "\n" + body
}

func renderChatCard(item chatEntry, width int) string {
	border := chatAssistantStyle
	switch item.Kind {
	case "user":
		border = chatUserStyle
	case "tool":
		border = chatAssistantStyle
	case "system":
		border = chatSystemStyle
	default:
		if item.Status == "final" {
			border = chatFinalStyle
		} else if item.Status == "thinking" || item.Status == "thinking_done" {
			border = chatThinkingStyle
		}
	}
	contentWidth := max(8, width-border.GetHorizontalFrameSize())
	rendered := border.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	if item.Kind != "tool" {
		return rendered
	}

	sep := lipgloss.NewStyle().Foreground(colorTool).Render("|")
	lines := strings.Split(rendered, "\n")
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = "  " + lines[i]
			continue
		}
		lines[i] = sep + " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func renderChatSection(item chatEntry, width int) string {
	title := cardTitleStyle.Foreground(colorAccent)
	bodyStyle := chatBodyStyle
	toolCallTitle := cardTitleStyle.Foreground(lipgloss.Color("#E5B567")).Bold(true)
	toolResultTitle := cardTitleStyle.Foreground(lipgloss.Color("#7AC7FF")).Bold(true)
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = cardTitleStyle.Foreground(colorUser)
	case "tool":
		if strings.HasPrefix(displayTitle, "Tool Result | ") {
			title = toolResultTitle
		} else {
			title = toolCallTitle
		}
		bodyStyle = toolBodyStyle
		status = ""
	case "system":
		title = cardTitleStyle.Foreground(colorMuted)
	default:
		if item.Status == "thinking" || item.Status == "thinking_done" {
			if item.Status == "thinking_done" {
				title = cardTitleStyle.Foreground(colorThinkingDone)
				bodyStyle = thinkingDoneBodyStyle
			} else {
				title = cardTitleStyle.Foreground(colorThinking)
				bodyStyle = thinkingBodyStyle
			}
			displayTitle = ""
			status = ""
		} else if item.Status == "final" {
			displayTitle = "FINAL ANSWER"
			title = cardTitleStyle.Foreground(colorFinalTitle).Bold(true)
		}
	}
	if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "thinking_done") {
		body := strings.TrimRight(wrapPlainText(item.Body, width), "\n")
		if item.Status == "thinking_done" {
			body = collapseThinkingBody(body)
		}
		return bodyStyle.Width(width).Render(body)
	}
	if item.Kind == "user" {
		return bodyStyle.Width(width).Render(formatChatBody(item, width))
	}
	headContent := title.Render(displayTitle)
	if status != "" {
		headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, mutedStyle.Render("  "+status))
	}
	head := lipgloss.NewStyle().
		Width(width).
		Render(headContent)
	if item.Kind == "tool" && strings.TrimSpace(item.Body) == "" {
		return head
	}
	body := bodyStyle.Width(width).Render(formatChatBody(item, width))
	if item.Kind == "assistant" && item.Status == "final" && strings.TrimSpace(item.Meta) != "" {
		meta := mutedStyle.Render(strings.TrimSpace(item.Meta))
		return lipgloss.JoinVertical(lipgloss.Left, head, body, meta)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, body)
}

func collapseThinkingBody(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) <= 2 {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:2], "\n") + "\n" + mutedStyle.Render("(trace collapsed)")
}
