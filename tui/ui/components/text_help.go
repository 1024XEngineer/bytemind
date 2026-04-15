package tui

import "strings"

func isHelpMarkdownText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "# Bytemind Help")
}

func renderHelpMarkdown(text string, width int) string {
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
