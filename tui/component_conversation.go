package tui

import (
	"fmt"
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
			resolvedItem := item
			if strings.Contains(item.Body, "[Paste #") || strings.Contains(item.Body, "[Pasted #") {
				resolvedItem.Body = m.resolveUserBodyPastes(item.Body)
			}
			blocks = append(blocks, renderChatRow(resolvedItem, width))
			i++
			continue
		}

		if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "thinking_done") {
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

		if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "thinking_done") {
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
	case "tool":
		label, name := toolDisplayParts(title)
		title = label
		if strings.TrimSpace(name) != "" {
			title += "  " + name
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
	bodyStyle := chatBodyBlockStyle
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = userMessageStyle
	case "tool":
		if strings.HasPrefix(strings.ToLower(displayTitle), "tool result | ") {
			title = toolResultTitleStyle
		} else {
			title = toolCallTitleStyle
		}
		if strings.EqualFold(status, "error") || strings.EqualFold(status, "warn") {
			bodyStyle = toolErrorBodyStyle
		} else {
			bodyStyle = toolBodyStyle
		}
	case "system":
		title = cardTitleStyle.Foreground(colorMuted)
		bodyStyle = chatMutedBodyBlockStyle
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
	if item.Kind == "tool" {
		label, _ := toolDisplayParts(displayTitle)
		headContent = renderToolTag(label, "info")
	}
	if item.Kind == "user" && strings.TrimSpace(item.Meta) != "" {
		headContent = chatHeaderMetaStyle.Render(item.Meta)
	}
	if status != "" {
		statusBadgeText := status
		if item.Kind == "tool" {
			switch strings.TrimSpace(strings.ToLower(status)) {
			case "done", "success":
				statusBadgeText = "✓"
			}
		}
		headContent = lipgloss.JoinHorizontal(
			lipgloss.Left,
			headContent,
			"  ",
			renderToolTag(statusBadgeText, status),
		)
	}
	if item.Kind == "assistant" {
		if badge := renderAssistantPhaseBadge(item.Status); badge != "" {
			headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, "  ", badge)
		}
	}
	head := chatHeaderStyle.Copy().
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
	sectionGroups := collapseRunSectionGroups(items)
	sections := make([]string, 0, len(sectionGroups))
	for i, group := range sectionGroups {
		if i > 0 {
			sections = append(sections, renderRunSectionDivider(contentWidth))
		}
		sections = append(sections, renderRunSectionGroup(group, contentWidth))
	}
	return outer.Width(contentWidth).Render(strings.Join(sections, "\n"))
}

func collapseRunSectionGroups(items []chatEntry) [][]chatEntry {
	groups := make([][]chatEntry, 0, len(items))
	for i := 0; i < len(items); {
		item := items[i]
		name, ok := collapsibleParallelToolName(item)
		if !ok {
			groups = append(groups, []chatEntry{item})
			i++
			continue
		}

		j := i + 1
		group := []chatEntry{item}
		for j < len(items) {
			nextName, nextOK := collapsibleParallelToolName(items[j])
			if !nextOK || nextName != name {
				break
			}
			group = append(group, items[j])
			j++
		}
		groups = append(groups, group)
		i = j
	}
	return groups
}

func collapsibleParallelToolName(item chatEntry) (string, bool) {
	if item.Kind != "tool" {
		return "", false
	}
	_, name := toolDisplayParts(item.Title)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if toolDisplayLabel(name) != "READ" {
		return "", false
	}
	return name, true
}

func renderRunSectionGroup(group []chatEntry, width int) string {
	if len(group) == 0 {
		return ""
	}
	if len(group) == 1 {
		return renderRunSection(group[0], width)
	}

	_, name := toolDisplayParts(group[0].Title)
	synthetic := chatEntry{
		Kind:   "tool",
		Title:  fmt.Sprintf("%s x %d | %s", toolDisplayLabel(name), len(group), name),
		Body:   summarizeParallelToolGroup(group, name),
		Status: aggregateToolGroupStatus(group),
	}
	return renderRunSection(synthetic, width)
}

func renderRunSection(item chatEntry, width int) string {
	if item.Kind == "tool" {
		style := resolveToolRunSectionStyle(item.Status)
		contentWidth := max(8, width-style.GetHorizontalFrameSize())
		return style.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	}
	if item.Kind == "assistant" && item.Status == "final" {
		contentWidth := max(8, width-runAnswerSectionStyle.GetHorizontalFrameSize())
		return runAnswerSectionStyle.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	}
	return renderChatSection(item, width)
}

func summarizeParallelToolGroup(group []chatEntry, name string) string {
	if len(group) == 0 {
		return ""
	}
	if toolDisplayLabel(name) == "READ" {
		return summarizeParallelReadGroup(group)
	}
	return fmt.Sprintf("%d parallel %s calls", len(group), strings.ToLower(toolDisplayLabel(name)))
}

func summarizeParallelReadGroup(group []chatEntry) string {
	fileNames := make([]string, 0, len(group))
	for _, item := range group {
		summary := strings.TrimSpace(firstNonEmptyLine(item.Body))
		if summary == "" {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(summary, "Read "))
		if name == "" {
			name = summary
		}
		fileNames = append(fileNames, name)
	}
	if len(fileNames) == 0 {
		return fmt.Sprintf("Read %d files", len(group))
	}
	previewCount := min(3, len(fileNames))
	preview := strings.Join(fileNames[:previewCount], ", ")
	if len(fileNames) > previewCount {
		return fmt.Sprintf("Read %d files: %s +%d", len(fileNames), preview, len(fileNames)-previewCount)
	}
	return fmt.Sprintf("Read %d files: %s", len(fileNames), preview)
}

func aggregateToolGroupStatus(group []chatEntry) string {
	hasDone := false
	hasRunning := false
	hasWarn := false
	for _, item := range group {
		switch strings.TrimSpace(strings.ToLower(item.Status)) {
		case "error", "failed":
			return "error"
		case "warn", "warning", "pending":
			hasWarn = true
		case "running", "active":
			hasRunning = true
		case "done", "success":
			hasDone = true
		}
	}
	switch {
	case hasWarn:
		return "warn"
	case hasRunning:
		return "running"
	case hasDone:
		return "done"
	default:
		return strings.TrimSpace(group[0].Status)
	}
}

func renderRunSectionDivider(width int) string {
	if width <= 0 {
		return ""
	}
	return runSectionDividerStyle.Width(width).Render(strings.Repeat("-", width))
}

func renderRunSectionDividerLegacy(width int) string {
	if width <= 0 {
		return ""
	}
	return runSectionDividerStyle.Width(width).Render(strings.Repeat("─", width))
}

func resolveToolRunSectionStyle(status string) lipgloss.Style {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "done", "success":
		return runToolSuccessSectionStyle
	case "warn", "warning", "pending":
		return runToolWarningSectionStyle
	case "error", "failed":
		return runToolErrorSectionStyle
	default:
		return runToolSectionStyle
	}
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
		return renderPillBadge("Generating", "running")
	case "settling":
		return renderPillBadge("Finalizing", "pending")
	case "final":
		return renderPillBadge("Answer", "neutral")
	default:
		return ""
	}
}

func renderToolTag(text, tagType string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	style := lipgloss.NewStyle().Bold(true)
	switch strings.TrimSpace(strings.ToLower(tagType)) {
	case "active", "running", "accent", "info":
		style = style.Foreground(semanticColors.AccentSoft)
	case "success", "done":
		style = style.Foreground(semanticColors.Success)
	case "warning", "pending", "warn":
		style = style.Foreground(semanticColors.Warning)
	case "error", "failed", "danger":
		style = style.Foreground(semanticColors.Danger)
	default:
		style = style.Foreground(semanticColors.TextMuted)
	}
	return style.Render(text)
}

func resolveRunCardStyle(items []chatEntry) lipgloss.Style {
	for _, item := range items {
		if item.Kind != "assistant" {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(item.Status)) {
		case "streaming":
			return runCardStreamingStyle
		case "settling":
			return runCardSettlingStyle
		}
	}
	return runCardStyle
}

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
