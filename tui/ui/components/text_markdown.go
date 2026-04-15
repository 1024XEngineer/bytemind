package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

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
	switch level {
	case 1:
		style = assistantHeading1Style
	case 2:
		style = assistantHeading2Style
	}
	wrapped := strings.Split(wrapPlainText(text, width), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, style.Render(part))
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
