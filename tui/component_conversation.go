package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderConversation() string {
	if len(m.chatItems) == 0 {
		return mutedStyle.Render("No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\".")
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatRow(item, width))
			i++
			continue
		}

		if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "thinking_done") {
			if rendered := m.renderThinkingRow(item, width); strings.TrimSpace(rendered) != "" {
				blocks = append(blocks, rendered)
			}
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}
		blocks = append(blocks, renderBytemindRunRow(m.chatItems[i:j], width))
		i = j
	}

	finalBlocks := make([]string, 0, len(blocks)*2)
	for i, block := range blocks {
		finalBlocks = append(finalBlocks, block)
		if i < len(blocks)-1 {
			finalBlocks = append(finalBlocks, messageSeparatorStyle.Render(""))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, finalBlocks...)
}

func (m model) renderConversationCopy() string {
	if len(m.chatItems) == 0 {
		return "No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\"."
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatCopySection(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}

		runParts := make([]string, 0, j-i)
		for _, runItem := range m.chatItems[i:j] {
			runParts = append(runParts, renderChatCopySection(runItem, width))
		}
		blocks = append(blocks, strings.Join(runParts, "\n\n"))
		i = j
	}
	return strings.Join(blocks, "\n\n")
}

func renderChatCopySection(item chatEntry, width int) string {
	title := strings.TrimSpace(item.Title)
	status := strings.TrimSpace(item.Status)
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "assistant":
		if strings.EqualFold(item.Status, "thinking") || strings.EqualFold(item.Status, "thinking_done") {
			title = "thinking"
			status = ""
		}
	case "user":
		if strings.TrimSpace(item.Meta) != "" {
			title = strings.TrimSpace(item.Meta)
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
	if item.Kind == "tool" && strings.TrimSpace(body) == "" {
		return title
	}
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
		if item.Status == "thinking" || item.Status == "thinking_done" {
			border = chatThinkingStyle
		} else if item.Status == "streaming" {
			border = chatStreamingStyle
		} else if item.Status == "settling" {
			border = chatSettlingStyle
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
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = userMessageStyle
	case "tool":
		if strings.HasPrefix(displayTitle, "Tool Result | ") {
			title = toolResultTitleStyle
		} else {
			title = toolCallTitleStyle
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
				title = cardTitleStyle.Foreground(colorThinkingBlue)
				bodyStyle = thinkingBodyStyle
			}
			displayTitle = "thinking"
			status = ""
		} else if item.Status == "streaming" {
			title = assistantStreamingTitleStyle
			displayTitle = assistantLabel
			status = ""
		} else if item.Status == "settling" {
			title = assistantSettlingTitleStyle
			displayTitle = assistantLabel
			status = ""
		} else if item.Status == "final" {
			title = assistantFinalTitleStyle
			displayTitle = assistantLabel
			status = ""
		} else {
			title = assistantMessageStyle
		}
	}
	headContent := title.Render(displayTitle)
	if item.Kind == "user" && strings.TrimSpace(item.Meta) != "" {
		headContent = mutedStyle.Copy().Faint(true).Render(item.Meta)
	}
	if status != "" {
		headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, mutedStyle.Render("  "+status))
	}
	if item.Kind == "assistant" {
		if badge := renderAssistantPhaseBadge(item.Status); badge != "" {
			headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, "  ", badge)
		}
	}
	head := lipgloss.NewStyle().
		Width(width).
		Render(headContent)
	if item.Kind == "tool" && strings.TrimSpace(item.Body) == "" {
		return head
	}
	body := bodyStyle.Width(width).Render(formatChatBody(item, width))
	return lipgloss.JoinVertical(lipgloss.Left, head, body)
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := chatBubbleWidth(item, width)
	card := renderChatCard(item, bubbleWidth)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunRow(items []chatEntry, width int) string {
	if len(items) == 0 {
		return ""
	}
	card := renderBytemindRunCard(items, width)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunCard(items []chatEntry, width int) string {
	outer := resolveRunCardStyle(items)
	contentWidth := max(8, width-outer.GetHorizontalFrameSize())
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, renderChatSection(item, contentWidth))
	}
	return outer.Width(contentWidth).Render(strings.Join(sections, "\n"))
}

func (m model) renderThinkingRow(item chatEntry, width int) string {
	panelWidth := max(24, width)

	bodyText := strings.TrimSpace(item.Body)
	if bodyText == "" && item.Status == "thinking_done" {
		bodyText = "Synthesis complete"
	}

	titleStyle := thinkingIndicatorStyle
	bodyStyle := thinkingDetailStyle
	if item.Status == "thinking_done" {
		titleStyle = cardTitleStyle.Foreground(colorThinkingDone)
		bodyStyle = thinkingDoneBodyStyle
	}

	parts := []string{titleStyle.Render(m.renderThinkingHeadline(item.Status))}
	if bodyText != "" {
		bodyWidth := max(8, panelWidth-2)
		bodyLines := strings.Split(wrapPlainText(bodyText, bodyWidth), "\n")
		for i := range bodyLines {
			bodyLines[i] = bodyStyle.Render(bodyLines[i])
		}
		parts = append(parts, lipgloss.JoinVertical(lipgloss.Left, bodyLines...))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, thinkingPanelStyle.Width(panelWidth).Render(body)))
}

func (m model) renderThinkingHeadline(status string) string {
	if status == "thinking_done" {
		return "thinking"
	}
	dots := []string{".", "..", "..."}
	frame := strings.TrimSpace(m.spinner.View())
	index := 0
	if frame != "" {
		sum := 0
		for _, r := range frame {
			sum += int(r)
		}
		index = sum % len(dots)
	}
	return "thinking" + dots[index]
}

func renderAssistantPhaseBadge(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "streaming":
		return statusGeneratingStyle.Render("Generating")
	case "settling":
		return statusSettlingStyle.Render("Finalizing")
	case "final":
		return statusFinalStyle.Render("Answer")
	default:
		return ""
	}
}

func resolveRunCardStyle(items []chatEntry) lipgloss.Style {
	for _, item := range items {
		if item.Kind != "assistant" {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(item.Status)) {
		case "streaming":
			return chatStreamingStyle
		case "settling":
			return chatSettlingStyle
		}
	}
	return chatAssistantStyle
}

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
