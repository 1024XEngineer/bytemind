package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func formatChatBody(item chatEntry, width int) string {
	return formatChatBodyMode(item, width, false)
}

func formatChatCopyBody(item chatEntry, width int) string {
	return formatChatBodyMode(item, width, true)
}

func formatChatBodyMode(item chatEntry, width int, copyMode bool) string {
	text := strings.ReplaceAll(item.Body, "\r\n", "\n")
	if item.Kind == "user" {
		return strings.TrimRight(wrapPlainText(text, width), "\n")
	}
	if item.Kind == "tool" {
		if !copyMode {
			text = firstNonEmptyLine(text)
		}
		if copyMode {
			return strings.TrimRight(renderToolCopyBody(text, width), "\n")
		}
		return strings.TrimRight(renderToolBody(text, width), "\n")
	}
	if item.Kind != "assistant" {
		return strings.TrimRight(renderSemanticPlainBody(text, width), "\n")
	}
	if isHelpMarkdownText(text) {
		if copyMode {
			return strings.TrimRight(renderHelpMarkdownCopy(text, width), "\n")
		}
		return strings.TrimRight(renderHelpMarkdown(text, width), "\n")
	}
	if copyMode {
		return strings.TrimRight(renderAssistantCopyBody(text, width), "\n")
	}
	return strings.TrimRight(renderAssistantBody(text, width), "\n")
}

func isHelpMarkdownText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "# Bytemind Help")
}

func renderHelpMarkdown(text string, width int) string {
	return renderHelpMarkdownLegacy(text, width)
}

func renderHelpMarkdownCopy(text string, width int) string {
	return stripANSI(renderHelpMarkdownLegacy(text, width))
}

func renderAssistantCopyBody(text string, width int) string {
	text = stripAssistantStructuralTags(text)
	result := renderStructuredMarkdown(markdownSurfaceAssistant, text, width)
	if strings.TrimSpace(result.Copy) != "" {
		return result.Copy
	}
	return stripANSI(renderAssistantBodyLegacy(text, width))
}

func renderHelpMarkdownLegacy(text string, width int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	prevBlank := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}

		switch {
		case isMarkdownHeading(trimmed):
			out = append(out, renderMarkdownHeading(trimmed, width))
		case isMarkdownListItem(trimmed):
			out = append(out, renderMarkdownListItem(trimmed, width))
		case strings.HasPrefix(trimmed, "> "):
			out = append(out, renderMarkdownQuote(trimmed, width))
		default:
			plainLine := normalizeAssistantMarkdownLine(line)
			if strings.TrimSpace(plainLine) == "" {
				if !prevBlank {
					out = append(out, "")
				}
				prevBlank = true
				continue
			}
			out = append(out, wrapPlainText(plainLine, width))
		}
		prevBlank = false
	}
	return strings.Join(out, "\n")
}

func renderToolBody(text string, width int) string {
	if toolTextLooksMarkdown(text) {
		result := renderStructuredMarkdown(markdownSurfaceTool, text, width)
		if strings.TrimSpace(result.Display) != "" {
			return result.Display
		}
	}
	return renderToolBodyLegacy(text, width)
}

func renderToolCopyBody(text string, width int) string {
	if toolTextLooksMarkdown(text) {
		result := renderStructuredMarkdown(markdownSurfaceTool, text, width)
		if strings.TrimSpace(result.Copy) != "" {
			return result.Copy
		}
	}
	return stripANSI(renderToolBodyLegacy(text, width))
}

func renderToolBodyLegacy(text string, width int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	prevBlank := true
	visualLine := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}
		out = append(out, renderToolLine(line, width, visualLine == 0))
		visualLine++
		prevBlank = false
	}
	return strings.Join(out, "\n")
}

func firstNonEmptyLine(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func renderToolLine(line string, width int, first bool) string {
	trimmed := strings.TrimSpace(line)
	contentWidth := max(8, width)
	switch {
	case isToolErrorLine(trimmed) && first:
		return renderStyledWrappedLine(trimmed, contentWidth, toolErrorSummaryStyle)
	case isToolSearchSummaryLine(trimmed):
		return renderStyledWrappedLine(trimmed, contentWidth, toolSearchSummaryStyle)
	case first:
		return renderStyledWrappedLine(trimmed, contentWidth, toolSummaryStyle)
	case isToolErrorLine(trimmed):
		return renderStyledWrappedLine(trimmed, contentWidth, toolErrorDetailStyle)
	case isToolSearchMatchLine(trimmed):
		return renderStyledWrappedLine(trimmed, contentWidth, toolSearchMatchStyle)
	case isToolMetaLine(trimmed):
		return renderStyledWrappedLine(trimmed, contentWidth, toolMetaStyle)
	default:
		return renderStyledWrappedLine(trimmed, contentWidth, toolDetailStyle)
	}
}

func renderStyledWrappedLine(line string, width int, style lipgloss.Style) string {
	wrapped := strings.Split(wrapPlainText(line, width), "\n")
	for i := range wrapped {
		wrapped[i] = style.Render(wrapped[i])
	}
	return strings.Join(wrapped, "\n")
}

func isToolSearchSummaryLine(line string) bool {
	return strings.HasPrefix(line, "Found ") && strings.Contains(line, " match(es) for ")
}

func isToolSearchMatchLine(line string) bool {
	fieldEnd := strings.IndexByte(line, ' ')
	if fieldEnd <= 0 {
		return false
	}
	location := line[:fieldEnd]
	colon := strings.LastIndex(location, ":")
	if colon <= 0 || colon == len(location)-1 {
		return false
	}
	for _, r := range location[colon+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	path := location[:colon]
	return strings.Contains(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, ".")
}

func isToolErrorLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "error") || strings.HasPrefix(lower, "stderr:")
}

func isToolMetaLine(line string) bool {
	if isToolSearchMatchLine(line) {
		return false
	}
	return strings.Contains(line, ": ")
}

func toolTextLooksMarkdown(text string) bool {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "```"):
			return true
		case strings.HasPrefix(trimmed, "> "):
			return true
		case isMarkdownHeading(trimmed):
			return true
		case isMarkdownListItem(trimmed):
			return true
		case strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|"):
			return true
		case strings.Contains(trimmed, "`"):
			return true
		case strings.Contains(trimmed, "[") && strings.Contains(trimmed, "]("):
			return true
		}
	}
	return false
}

func renderSemanticPlainBody(text string, width int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	prevBlank := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}
		rendered := renderSemanticAssistantLine(line, width)
		out = append(out, rendered)
		prevBlank = false
	}
	return strings.Join(out, "\n")
}

func wrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		for _, part := range wrapLineSmart(line, width) {
			wrapped = append(wrapped, strings.TrimRight(part, " "))
		}
	}
	return strings.Join(wrapped, "\n")
}

func wrapLineSmart(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}

	out := make([]string, 0, 4)
	start := 0
	for start < len(runes) {
		curWidth := 0
		end := start
		lastSpaceEnd := -1

		for i := start; i < len(runes); i++ {
			rw := runewidth.RuneWidth(runes[i])
			if rw < 0 {
				rw = 0
			}
			if curWidth+rw > width {
				break
			}
			curWidth += rw
			end = i + 1
			if unicode.IsSpace(runes[i]) {
				lastSpaceEnd = i + 1
			}
		}

		if end == start {
			// Fallback for extra-wide single rune.
			end = start + 1
		} else if lastSpaceEnd > start && end < len(runes) {
			end = lastSpaceEnd
		}

		segment := strings.TrimRightFunc(string(runes[start:end]), unicode.IsSpace)
		if segment == "" {
			segment = string(runes[start:end])
		}
		out = append(out, segment)
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}

	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func tidyAssistantSpacing(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+4)
	inCodeBlock := false
	prevBlank := true

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, line)
			inCodeBlock = !inCodeBlock
			prevBlank = false
			continue
		}

		if inCodeBlock {
			out = append(out, line)
			prevBlank = trimmed == ""
			continue
		}

		if trimmed == "" {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}

		if needsLeadingBlankLine(trimmed) && !prevBlank && len(out) > 0 {
			out = append(out, "")
		}

		out = append(out, line)
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

func needsLeadingBlankLine(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "> ") {
		return true
	}
	if len(line) >= 3 && line[1] == '.' && line[2] == ' ' && line[0] >= '0' && line[0] <= '9' {
		return true
	}
	return false
}

func renderAssistantBodyLegacy(text string, width int) string {
	text = stripAssistantStructuralTags(text)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inCodeBlock := false
	prevBlank := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			codeLine := strings.TrimRight(wrapPlainText(line, width), "\n")
			if strings.TrimSpace(codeLine) == "" {
				if !prevBlank {
					out = append(out, "")
				}
				prevBlank = true
				continue
			}
			out = append(out, codeStyle.Render(codeLine))
			prevBlank = false
			continue
		}

		plainLine := normalizeAssistantMarkdownLine(line)
		if strings.TrimSpace(plainLine) == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}

		switch {
		case isMarkdownHeading(strings.TrimSpace(plainLine)):
			out = append(out, renderMarkdownHeading(strings.TrimSpace(plainLine), width))
		case strings.HasPrefix(strings.TrimSpace(plainLine), "> "):
			out = append(out, renderMarkdownQuote(strings.TrimSpace(plainLine), width))
		default:
			out = append(out, renderSemanticAssistantLine(plainLine, width))
		}
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

func stripAssistantStructuralTags(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	filtered := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "<proposed_plan>" || trimmed == "</proposed_plan>" {
			continue
		}
		if trimmed == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			filtered = append(filtered, "")
			continue
		}
		lastBlank = false
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func renderSemanticAssistantLine(line string, width int) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	listPrefix, core, isList := splitSemanticListPrefix(trimmed)
	if core == "" {
		core = trimmed
	}

	if isDocumentTitleLine(core) {
		wrapped := strings.Split(wrapPlainText(core, width), "\n")
		for i := range wrapped {
			wrapped[i] = assistantHeading1Style.Render(wrapped[i])
		}
		return joinWithListPrefix(listPrefix, wrapped, isList)
	}

	if isStageTitleLine(core) {
		wrapped := strings.Split(wrapPlainText(core, max(8, width-runewidth.StringWidth(listPrefix))), "\n")
		for i := range wrapped {
			wrapped[i] = assistantHeading2Style.Render(wrapped[i])
		}
		wrapped = applyIntentStyleToLines(core, wrapped)
		return joinWithListPrefix(listPrefix, wrapped, isList)
	}

	if isSectionTitleLine(core) {
		wrapped := strings.Split(wrapPlainText(core, max(8, width-runewidth.StringWidth(listPrefix))), "\n")
		for i := range wrapped {
			wrapped[i] = accentStyle.Render(wrapped[i])
		}
		wrapped = applyIntentStyleToLines(core, wrapped)
		return joinWithListPrefix(listPrefix, wrapped, isList)
	}

	label, rest, ok := splitSemanticLabel(core)
	if !ok {
		wrapped := strings.Split(wrapPlainText(core, max(8, width-runewidth.StringWidth(listPrefix))), "\n")
		wrapped = applyIntentStyleToLines(core, wrapped)
		return joinWithListPrefix(listPrefix, wrapped, isList)
	}
	prefix := accentStyle.Render(label)
	if rest == "" {
		lines := []string{prefix}
		lines = applyIntentStyleToLines(core, lines)
		return joinWithListPrefix(listPrefix, lines, isList)
	}

	prefixWidth := runewidth.StringWidth(listPrefix) + runewidth.StringWidth(label+" ")
	contentWidth := max(8, width-prefixWidth)
	wrapped := strings.Split(wrapPlainText(rest, contentWidth), "\n")
	lines := make([]string, 0, len(wrapped))
	for i, part := range wrapped {
		if i == 0 {
			lines = append(lines, prefix+" "+part)
			continue
		}
		lines = append(lines, strings.Repeat(" ", runewidth.StringWidth(label+" "))+part)
	}
	lines = applyIntentStyleToLines(core, lines)
	return joinWithListPrefix(listPrefix, lines, isList)
}

func isStageTitleLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "\u7b2c") && strings.Contains(trimmed, "\u9636\u6bb5") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "phase ")
}

func isSectionTitleLine(line string) bool {
	line = strings.TrimSpace(line)
	if !(strings.HasSuffix(line, ":") || strings.HasSuffix(line, "\uff1a")) {
		return false
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line, "\uff1a"), ":"))
	if body == "" || runewidth.StringWidth(body) > 20 {
		return false
	}
	return !strings.Contains(body, " ")
}

func splitSemanticLabel(line string) (label string, rest string, ok bool) {
	sep := "\uff1a"
	idx := strings.Index(line, sep)
	if idx < 0 {
		sep = ":"
		idx = strings.Index(line, sep)
	}
	if idx <= 0 {
		return "", "", false
	}
	left := strings.TrimSpace(line[:idx])
	right := strings.TrimSpace(line[idx+len(sep):])
	if left == "" || runewidth.StringWidth(left) > 10 {
		return "", "", false
	}
	if strings.Contains(left, " ") {
		return "", "", false
	}
	return left + sep, right, true
}

func applyLineIntentStyle(rawLine, renderedLine string) string {
	switch semanticIntent(rawLine) {
	case "warning":
		return warnStyle.Render(renderedLine)
	case "error":
		return errorStyle.Render(renderedLine)
	case "success":
		return doneStyle.Render(renderedLine)
	case "info":
		return infoStyle.Render(renderedLine)
	default:
		return renderedLine
	}
}

func semanticIntent(rawLine string) string {
	line := strings.TrimSpace(strings.ToLower(rawLine))
	switch {
	case hasSemanticLabel(line, "\u6ce8\u610f", "\u8b66\u544a", "warning", "warn", "caution"), hasAnyPrefix(line, "! "):
		return "warning"
	case hasSemanticLabel(line, "\u9519\u8bef", "\u5931\u8d25", "error", "fatal", "failure"), hasAnyPrefix(line, "x "):
		return "error"
	case hasSemanticLabel(line, "\u6210\u529f", "\u5b8c\u6210", "success", "done", "ok"):
		return "success"
	case hasSemanticLabel(line, "\u63d0\u793a", "\u8bf4\u660e", "\u4fe1\u606f", "info", "note", "hint", "tip"):
		return "info"
	default:
		return ""
	}
}

func hasSemanticLabel(line string, labels ...string) bool {
	for _, label := range labels {
		if line == label ||
			strings.HasPrefix(line, label+":") ||
			strings.HasPrefix(line, label+"\uff1a") ||
			strings.HasPrefix(line, label+" ") {
			return true
		}
	}
	return false
}

func applyIntentStyleToLines(rawLine string, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i := range lines {
		out[i] = applyLineIntentStyle(rawLine, lines[i])
	}
	return out
}

func splitSemanticListPrefix(line string) (prefix string, core string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return trimmed[:2], strings.TrimSpace(trimmed[2:]), true
	}
	if marker, rest, found := splitOrderedListItem(trimmed); found {
		return marker + " ", strings.TrimSpace(rest), true
	}
	return "", trimmed, false
}

func joinWithListPrefix(prefix string, lines []string, isList bool) string {
	if len(lines) == 0 {
		return ""
	}
	if !isList || strings.TrimSpace(prefix) == "" {
		return strings.Join(lines, "\n")
	}
	prefixGlyph := strings.TrimSpace(prefix)
	prefixIndent := strings.Repeat(" ", runewidth.StringWidth(prefix))
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			out = append(out, prefixGlyph+" "+line)
			continue
		}
		out = append(out, prefixIndent+line)
	}
	return strings.Join(out, "\n")
}

func isDocumentTitleLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.ContainsAny(line, ":\uff1a.!?\uff1f\uff01") {
		return false
	}
	if runewidth.StringWidth(line) > 24 {
		return false
	}
	for _, suffix := range []string{"\u603b\u89c8", "\u603b\u7ed3", "\u6982\u89c8", "\u8ba1\u5212", "\u6e05\u5355", "\u8bf4\u660e", "Overview", "Summary", "Plan", "Checklist", "Notes"} {
		if strings.HasSuffix(line, suffix) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

var assistantInlineTokenReplacer = strings.NewReplacer(
	"**", "",
	"__", "",
	"~~", "",
	"`", "",
)

func normalizeAssistantMarkdownLine(line string) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentWidth]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	for strings.HasPrefix(trimmed, ">") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	}
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level > 0 && (level == len(trimmed) || trimmed[level] == ' ') {
			trimmed = strings.TrimSpace(trimmed[level:])
		}
	}

	if isMarkdownTableDivider(trimmed) {
		return ""
	}

	prefix := ""
	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		prefix = "- [ ] "
		trimmed = strings.TrimSpace(trimmed[len("- [ ] "):])
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		prefix = "- [x] "
		trimmed = strings.TrimSpace(trimmed[len("- [x] "):])
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
		prefix = "- "
		trimmed = strings.TrimSpace(trimmed[2:])
	default:
		if marker, rest, ok := splitOrderedListItem(trimmed); ok {
			prefix = marker + " "
			trimmed = rest
		}
	}

	if looksLikeMarkdownTable(trimmed) {
		parts := make([]string, 0, 8)
		for _, cell := range strings.Split(trimmed, "|") {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				continue
			}
			parts = append(parts, cell)
		}
		trimmed = strings.Join(parts, " | ")
	}

	trimmed = stripMarkdownLinks(trimmed)
	trimmed = assistantInlineTokenReplacer.Replace(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	return indent + prefix + trimmed
}

func splitOrderedListItem(line string) (marker string, rest string, ok bool) {
	if len(line) < 3 {
		return "", "", false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 0 || len(line) <= index+1 || line[index] != '.' || line[index+1] != ' ' {
		return "", "", false
	}
	return line[:index+1], strings.TrimSpace(line[index+2:]), true
}

func isMarkdownTableDivider(line string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if compact == "" || strings.Count(compact, "|") < 1 {
		return false
	}
	for _, ch := range compact {
		switch ch {
		case '|', '-', ':':
		default:
			return false
		}
	}
	return true
}

func stripMarkdownLinks(line string) string {
	if line == "" {
		return line
	}

	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		start := -1
		isImage := false
		switch {
		case i+1 < len(line) && line[i] == '!' && line[i+1] == '[':
			start = i + 2
			isImage = true
		case line[i] == '[':
			start = i + 1
		}

		if start < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}

		mid := strings.Index(line[start:], "](")
		if mid < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		textEnd := start + mid
		urlStart := textEnd + 2
		urlEndRel := strings.IndexByte(line[urlStart:], ')')
		if urlEndRel < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		urlEnd := urlStart + urlEndRel
		label := strings.TrimSpace(line[start:textEnd])
		url := strings.TrimSpace(line[urlStart:urlEnd])
		if label != "" {
			b.WriteString(label)
		}
		if url != "" {
			if !isImage {
				b.WriteString(" (")
				b.WriteString(url)
				b.WriteString(")")
			}
		}
		i = urlEnd + 1
	}
	return b.String()
}

func isMarkdownHeading(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ")
}

func renderMarkdownHeading(line string, width int) string {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	text := strings.TrimSpace(line[level:])
	style := assistantHeading3Style
	prefix := "\u25b8 "
	switch level {
	case 1:
		style = assistantHeading1Style
		prefix = "\u258d "
	case 2:
		style = assistantHeading2Style
		prefix = "\u25c6 "
	}
	contentWidth := max(8, width-runewidth.StringWidth(prefix))
	wrapped := strings.Split(wrapPlainText(text, contentWidth), "\n")
	rendered := make([]string, 0, len(wrapped))
	for i, part := range wrapped {
		if i == 0 {
			rendered = append(rendered, style.Render(prefix+part))
			continue
		}
		rendered = append(rendered, style.Render(strings.Repeat(" ", runewidth.StringWidth(prefix))+part))
	}
	return strings.Join(rendered, "\n")
}

func isMarkdownListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	return isOrderedListItem(line)
}

func isOrderedListItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	return index > 0 && len(line) > index+1 && line[index] == '.' && line[index+1] == ' '
}

func renderMarkdownListItem(line string, width int) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " "))
	indent := strings.Repeat(" ", indentWidth)
	trimmed := strings.TrimSpace(line)
	marker := ""
	content := ""

	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		marker = trimmed[:1]
		content = strings.TrimSpace(trimmed[2:])
	default:
		for i := 0; i < len(trimmed); i++ {
			if trimmed[i] == '.' && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				marker = trimmed[:i+1]
				content = strings.TrimSpace(trimmed[i+2:])
				break
			}
		}
	}

	if content == "" {
		content = trimmed
	}

	prefix := indent + marker + " "
	contentWidth := max(8, width-runewidth.StringWidth(prefix))
	wrapped := strings.Split(wrapPlainText(content, contentWidth), "\n")
	lines := make([]string, 0, len(wrapped))
	for i, part := range wrapped {
		if i == 0 {
			lines = append(lines, indent+listMarkerStyle.Render(marker)+" "+part)
			continue
		}
		lines = append(lines, indent+strings.Repeat(" ", runewidth.StringWidth(marker))+" "+part)
	}
	return strings.Join(lines, "\n")
}

func renderMarkdownQuote(line string, width int) string {
	content := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	wrapped := strings.Split(wrapPlainText(content, max(8, width-2)), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, quoteLineStyle.Render(part))
	}
	return strings.Join(rendered, "\n")
}

func looksLikeMarkdownTable(line string) bool {
	return strings.Count(line, "|") >= 2
}
