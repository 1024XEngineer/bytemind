package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	toolIcon     = "●"
	toolTreeChar = "└"
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
	runningIndicatorVisible := m.runningToolIndicatorVisible()
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

		if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "thinking_done") && !m.shouldShowThinkingRowInConversation(item) {
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}
		blocks = append(blocks, renderBytemindRunRow(m.chatItems[i:j], width, m.toolDetailExpanded, runningIndicatorVisible))
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

func (m model) runningToolIndicatorVisible() bool {
	frame := strings.TrimSpace(m.spinner.View())
	if frame == "" {
		return true
	}
	sum := 0
	for _, r := range frame {
		sum += int(r)
	}
	return sum%2 == 0
}

func (m model) shouldShowThinkingRowInConversation(item chatEntry) bool {
	if !m.subAgentPending {
		return false
	}
	if item.Kind != "assistant" {
		return false
	}
	if item.Status != "thinking" && item.Status != "thinking_done" {
		return false
	}
	body := strings.ToLower(strings.TrimSpace(item.Body))
	return strings.Contains(body, "task:") || strings.Contains(body, "running subagent")
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
	// Do NOT apply border.Width() — renderChatSection already wraps head and
	// body to contentWidth. Applying .Width() again causes lipgloss to re-wrap
	// at word boundaries. Instead, subtract the border's horizontal padding so
	// the total rendered width stays the same.
	borderPadding := border.GetHorizontalPadding()
	sectionWidth := max(8, contentWidth-borderPadding)
	rendered := border.Render(renderChatSection(item, sectionWidth))
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

func renderBytemindRunRow(items []chatEntry, width int, toolDetailsExpanded bool, runningIndicatorVisible bool) string {
	if len(items) == 0 {
		return ""
	}
	card := renderBytemindRunCard(items, width, toolDetailsExpanded, runningIndicatorVisible)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunCard(items []chatEntry, width int, toolDetailsExpanded bool, runningIndicatorVisible bool) string {
	outer := resolveRunCardStyle(items)
	contentWidth := max(8, width-outer.GetHorizontalFrameSize())
	sectionGroups := collapseRunSectionGroupsForView(items, toolDetailsExpanded)
	sections := make([]string, 0, len(sectionGroups))
	for _, group := range sectionGroups {
		sections = append(sections, renderRunSectionGroup(group, contentWidth, toolDetailsExpanded, runningIndicatorVisible))
	}
	// Do NOT apply outer.Width() here — each section already manages its own
	// width via .Width() inside renderRunSectionGroup. Applying .Width() again
	// on the joined output causes lipgloss to re-wrap at word boundaries,
	// breaking the formatting that the inner sections carefully constructed.
	return outer.Render(strings.Join(sections, "\n"))
}

func collapseRunSectionGroupsForView(items []chatEntry, toolDetailsExpanded bool) [][]chatEntry {
	if toolDetailsExpanded {
		return collapseRunSectionGroups(items)
	}
	return collapseRunSectionGroupsCollapsedLive(items)
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

func collapseRunSectionGroupsCollapsedLive(items []chatEntry) [][]chatEntry {
	groups := make([][]chatEntry, 0, len(items))
	for i := 0; i < len(items); {
		item := items[i]
		key, ok := collapsibleParallelGroupKey(item, false)
		if !ok {
			groups = append(groups, []chatEntry{item})
			i++
			continue
		}

		j := i + 1
		group := []chatEntry{item}
		for j < len(items) {
			nextKey, nextOK := collapsibleParallelGroupKey(items[j], false)
			if !nextOK || nextKey != key {
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

func collapsibleParallelGroupKey(item chatEntry, toolDetailsExpanded bool) (string, bool) {
	name, ok := collapsibleParallelToolName(item)
	if !ok {
		return "", false
	}
	if !toolDetailsExpanded && isLiveInspectToolName(name) {
		return "inspect_live", true
	}
	return name, true
}

// collapsibleParallelToolName returns the tool name if this entry can be
// grouped with adjacent entries of the same tool. All tool entries are
// collapsible; grouping happens at render time, not data time.
func collapsibleParallelToolName(item chatEntry) (string, bool) {
	if item.Kind != "tool" {
		return "", false
	}
	_, name := toolDisplayParts(item.Title)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	return name, true
}

func renderRunSectionGroup(group []chatEntry, width int, toolDetailsExpanded bool, runningIndicatorVisible bool) string {
	if len(group) == 0 {
		return ""
	}
	if len(group) == 1 {
		return renderRunSection(group[0], width, toolDetailsExpanded, runningIndicatorVisible)
	}
	if !toolDetailsExpanded && isLiveInspectGroup(group) {
		return renderLiveInspectGroup(group, width, runningIndicatorVisible)
	}

	_, name := toolDisplayParts(group[0].Title)
	renderer := GetToolRenderer(name)
	label := toolDisplayLabel(name)
	if renderer != nil {
		label = renderer.DisplayLabel()
	}

	summaryLine := summarizeParallelToolGroup(group, name)
	status := aggregateToolGroupStatus(group)

	// Tree-style summary line plus detail lines for each entry.
	statusBadge := ""
	if shouldRenderToolStatusTag(status) {
		statusBadge = renderToolTag(status, status)
	}

	headLine := toolStatusIndicator(status, runningIndicatorVisible) + " " + label
	if summaryLine != "" {
		headLine += " " + summaryLine
	}
	if statusBadge != "" {
		headLine += "  " + statusBadge
	}

	// Build detail lines from each entry's CompactBody.
	detailLines := make([]string, 0, len(group))
	connectorStyle := lipgloss.NewStyle().Foreground(colorTool)
	style := resolveToolRunSectionStyle(status)
	contentWidth := max(8, width-style.GetHorizontalFrameSize())
	// Available width for detail text: contentWidth - indent(2) - connector(1)
	maxDetail := max(12, contentWidth-3)
	for _, item := range group {
		compact := item.CompactBody
		if compact == "" {
			compact = strings.TrimSpace(firstNonEmptyLine(item.Body))
		}
		if compact == "" {
			continue
		}
		if runewidth.StringWidth(compact) > maxDetail {
			compact = runewidth.Truncate(compact, maxDetail, "…")
		}
		detailLines = append(detailLines, connectorStyle.Render(toolTreeChar)+compact)
	}

	indent := "  "

	// Always truncate headLine to fit within contentWidth so lipgloss
	// doesn't wrap it at an ugly word boundary.
	if runewidth.StringWidth(headLine) > contentWidth {
		headLine = runewidth.Truncate(headLine, contentWidth, "…")
	}

	body := headLine
	if toolDetailsExpanded && len(detailLines) > 0 {
		body = headLine + "\n" + indent + strings.Join(detailLines, "\n"+indent)
	}
	return style.Width(contentWidth).Render(body)
}

func renderRunSection(item chatEntry, width int, toolDetailsExpanded bool, runningIndicatorVisible bool) string {
	if item.Kind == "tool" {
		return renderToolTreeItem(item, width, toolDetailsExpanded, runningIndicatorVisible)
	}
	if item.Kind == "assistant" && item.Status == "final" {
		contentWidth := max(8, width-runAnswerSectionStyle.GetHorizontalFrameSize())
		return runAnswerSectionStyle.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	}
	return renderChatSection(item, width)
}

// renderToolTreeItem renders a single tool entry in tree style.
func renderToolTreeItem(item chatEntry, width int, toolDetailsExpanded bool, runningIndicatorVisible bool) string {
	_, name := toolDisplayParts(item.Title)
	renderer := GetToolRenderer(name)
	label := toolDisplayLabel(name)
	if renderer != nil {
		label = renderer.DisplayLabel()
	}

	compact := item.CompactBody
	if compact == "" {
		compact = strings.TrimSpace(firstNonEmptyLine(item.Body))
	}

	statusBadge := ""
	if shouldRenderToolStatusTag(item.Status) {
		statusBadge = renderToolTag(item.Status, item.Status)
	}

	headLine := toolStatusIndicator(item.Status, runningIndicatorVisible) + " " + label
	if compact != "" {
		headLine += " " + compact
	}
	if statusBadge != "" {
		headLine += "  " + statusBadge
	}

	style := resolveToolRunSectionStyle(item.Status)
	contentWidth := max(8, width-style.GetHorizontalFrameSize())

	// Always truncate headLine to fit within contentWidth so lipgloss
	// doesn't wrap it at an ugly word boundary.
	if runewidth.StringWidth(headLine) > contentWidth {
		headLine = runewidth.Truncate(headLine, contentWidth, "…")
	}

	indent := "  "
	body := headLine
	if len(item.DetailLines) > 0 {
		limit := len(item.DetailLines)
		if !toolDetailsExpanded && limit > 8 {
			limit = 8
		}
		detailLines := make([]string, 0, limit+1)
		for i, detail := range item.DetailLines {
			if i >= limit {
				detailLines = append(detailLines, toolExpandHintStyle.Render("(ctrl+o to expand)"))
				break
			}
			detail = strings.TrimSpace(detail)
			if detail == "" {
				continue
			}
			detailLines = append(detailLines, renderDiffDetailLine(detail))
		}
		if len(detailLines) > 0 {
			body = headLine + "\n" + indent + strings.Join(detailLines, "\n"+indent)
		}
	}

	return style.Width(contentWidth).Render(body)
}

func renderDiffDetailLine(line string) string {
	if len(line) < 2 {
		return line
	}
	switch line[0] {
	case '+':
		return toolDiffAddStyle.Render(line)
	case '-':
		return toolDiffRemoveStyle.Render(line)
	case ' ':
		return toolDiffContextStyle.Render(line)
	}
	return line
}

func summarizeParallelToolGroup(group []chatEntry, name string) string {
	if len(group) == 0 {
		return ""
	}
	renderer := GetToolRenderer(name)
	label := toolDisplayLabel(name)
	if renderer != nil {
		label = renderer.DisplayLabel()
	}
	// For READ tools, show file names; for others, show count
	if toolDisplayLabel(name) == "READ" {
		return summarizeParallelReadGroup(group)
	}
	return fmt.Sprintf("%d × %s", len(group), strings.ToLower(label))
}

func isLiveInspectToolName(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "search_text", "web_search", "read_file", "list_files":
		return true
	default:
		return false
	}
}

func isLiveInspectGroup(group []chatEntry) bool {
	if len(group) == 0 {
		return false
	}
	for _, item := range group {
		_, name := toolDisplayParts(item.Title)
		if !isLiveInspectToolName(name) {
			return false
		}
	}
	return true
}

func renderLiveInspectGroup(group []chatEntry, width int, runningIndicatorVisible bool) string {
	status := aggregateToolGroupStatus(group)
	style := resolveToolRunSectionStyle(status)
	contentWidth := max(8, width-style.GetHorizontalFrameSize())
	summary := summarizeLiveInspectGroup(group)
	hintSuffix := "(ctrl+o to expand)"
	statusTag := ""
	if shouldRenderToolStatusTag(status) && normalizeToolStatus(status) != "running" && normalizeToolStatus(status) != "active" {
		statusTag = renderToolTag(status, status)
	}
	// Reserve space for detail line prefix (indent + connector + space) when tool is running.
	reservedForDetail := 0
	if isToolGroupRunning(group) {
		if detail := latestLiveInspectHint(group); detail != "" {
			reservedForDetail = 4 // "  ├ " prefix width
		}
	}
	headLine := buildLiveInspectHeadline(summary, status, statusTag, contentWidth, hintSuffix, runningIndicatorVisible, reservedForDetail)

	if isToolGroupRunning(group) {
		if detail := latestLiveInspectHint(group); detail != "" {
			connectorStyle := lipgloss.NewStyle().Foreground(colorTool)
			maxDetailWidth := max(12, contentWidth-6)
			headLine += "\n  " + connectorStyle.Render(toolTreeChar) + " " + compact(detail, maxDetailWidth)
		}
	}

	return style.Width(contentWidth).Render(headLine)
}

func buildLiveInspectHeadline(summary, status, statusTag string, width int, hintSuffix string, runningIndicatorVisible bool, reservedForDetail int) string {
	indicator := toolStatusIndicator(status, runningIndicatorVisible) + " "
	hintText := toolExpandHintStyle.Render(hintSuffix)
	tagWidth := 0
	if statusTag != "" {
		tagWidth = 2 + lipgloss.Width(statusTag)
	}
	reservedWithHint := lipgloss.Width(indicator) + tagWidth + 1 + lipgloss.Width(hintText)
	available := width - reservedWithHint - reservedForDetail - 2
	showHint := true
	if available < 8 {
		showHint = false
		available = width - lipgloss.Width(indicator) - tagWidth - reservedForDetail - 1
	}
	available = max(1, available)
	primary := compact(summary, available)
	line := indicator + primary
	if statusTag != "" {
		line += "  " + statusTag
	}
	if showHint {
		line += " " + hintText
	}
	return line
}

func isToolGroupRunning(group []chatEntry) bool {
	for _, item := range group {
		switch normalizeToolStatus(item.Status) {
		case "running", "active":
			return true
		}
	}
	return false
}

func summarizeLiveInspectGroup(group []chatEntry) string {
	searchCount := 0
	readCount := 0
	listCount := 0
	running := false

	for _, item := range group {
		_, name := toolDisplayParts(item.Title)
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "search_text", "web_search":
			searchCount++
		case "read_file":
			readCount++
		case "list_files":
			listCount++
		}
		switch normalizeToolStatus(item.Status) {
		case "running", "active":
			running = true
		}
	}

	parts := make([]string, 0, 3)
	if searchCount > 0 {
		parts = append(parts, fmt.Sprintf("Searching for %d %s", searchCount, pluralWord(searchCount, "pattern", "patterns")))
	}
	if readCount > 0 {
		parts = append(parts, fmt.Sprintf("reading %d %s", readCount, pluralWord(readCount, "file", "files")))
	}
	if listCount > 0 {
		parts = append(parts, fmt.Sprintf("listing %d %s", listCount, pluralWord(listCount, "path", "paths")))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("Running %d tool calls", len(group)))
	}

	summary := strings.Join(parts, ", ")
	if running {
		return summary + "..."
	}
	return summary
}

func latestLiveInspectHint(group []chatEntry) string {
	for i := len(group) - 1; i >= 0; i-- {
		status := normalizeToolStatus(group[i].Status)
		if status != "running" && status != "active" {
			continue
		}
		if hint := compactToolHint(group[i]); hint != "" {
			return hint
		}
	}
	for i := len(group) - 1; i >= 0; i-- {
		if hint := compactToolHint(group[i]); hint != "" {
			return hint
		}
	}
	return ""
}

func compactToolHint(item chatEntry) string {
	hint := strings.TrimSpace(item.CompactBody)
	if hint == "" {
		hint = strings.TrimSpace(firstNonEmptyLine(item.Body))
	}
	if hint == "" {
		return ""
	}
	return hint
}

func pluralWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func summarizeParallelReadGroup(group []chatEntry) string {
	fileNames := make([]string, 0, len(group))
	for _, item := range group {
		// Prefer CompactBody (e.g. "model.go (1-50)")
		name := strings.TrimSpace(item.CompactBody)
		if name == "" {
			name = strings.TrimSpace(firstNonEmptyLine(item.Body))
		}
		if name == "" {
			continue
		}
		name = strings.TrimSpace(strings.TrimPrefix(name, "Read "))
		if name == "" {
			continue
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
	hasQueued := false
	hasWarn := false
	for _, item := range group {
		switch strings.TrimSpace(strings.ToLower(item.Status)) {
		case "error", "failed":
			return "error"
		case "warn", "warning", "pending":
			hasWarn = true
		case "running", "active":
			hasRunning = true
		case "queued":
			hasQueued = true
		case "done", "success":
			hasDone = true
		}
	}
	switch {
	case hasWarn:
		return "warn"
	case hasRunning:
		return "running"
	case hasQueued:
		return "queued"
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
	text := "thinking" + dots[index]
	if m.stalled {
		return lipgloss.NewStyle().Foreground(semanticColors.Warning).Render(text)
	}
	return text
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
	case "queued":
		style = style.Foreground(semanticColors.TextMuted)
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

func normalizeToolStatus(status string) string {
	return strings.TrimSpace(strings.ToLower(status))
}

func shouldRenderToolStatusTag(status string) bool {
	switch normalizeToolStatus(status) {
	case "", "done", "success":
		return false
	default:
		return true
	}
}

func toolStatusIndicator(status string, runningIndicatorVisible bool) string {
	glyph := toolIcon
	style := lipgloss.NewStyle()
	switch normalizeToolStatus(status) {
	case "running", "active":
		if runningIndicatorVisible {
			glyph = "●"
		} else {
			glyph = " "
		}
		style = style.Foreground(semanticColors.AccentSoft)
	case "queued":
		if runningIndicatorVisible {
			glyph = "○"
		} else {
			glyph = " "
		}
		style = style.Foreground(semanticColors.TextMuted)
	case "warn", "warning", "pending":
		style = style.Foreground(semanticColors.Warning)
	case "error", "failed":
		style = style.Foreground(semanticColors.Danger)
	case "done", "success":
		style = style.Foreground(semanticColors.Success)
	default:
		style = style.Foreground(colorTool)
	}
	return style.Render(glyph)
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
