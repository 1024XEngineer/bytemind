package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	codeCommentPattern = regexp.MustCompile(`(//.*$|#.*$)`)
	codeStringPattern  = regexp.MustCompile(`("(?:\\.|[^"])*"|'(?:\\.|[^'])*'|` + "`" + `[^` + "`" + `]*` + "`" + `)`)
	codeNumberPattern  = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
	codeKeywordPattern = regexp.MustCompile(`\b(func|package|import|return|if|else|switch|case|for|range|break|continue|go|defer|struct|interface|type|var|const|map|chan|select|true|false|nil|class|public|private|protected|static|new|try|catch|finally|throw|async|await|let|const|function|from|export|default|def|lambda|match|with|yield)\b`)
)

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

func renderAssistantBody(text string, width int) string {
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
			out = append(out, renderHighlightedCodeLine(codeLine))
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

func renderHighlightedCodeLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return codeStyle.Render("")
	}

	commentLoc := codeCommentPattern.FindStringIndex(line)
	commentStart := len(line)
	if commentLoc != nil {
		commentStart = commentLoc[0]
	}

	codePart := line[:commentStart]
	commentPart := ""
	if commentStart < len(line) {
		commentPart = line[commentStart:]
	}

	renderedCode := applyCodePattern(codePart, codeStringPattern, lipgloss.NewStyle().Foreground(colorCodeString))
	renderedCode = applyCodePattern(renderedCode, codeNumberPattern, lipgloss.NewStyle().Foreground(colorCodeNumber))
	renderedCode = applyCodePattern(renderedCode, codeKeywordPattern, lipgloss.NewStyle().Foreground(colorCodeKeyword))

	if commentPart != "" {
		renderedCode += lipgloss.NewStyle().Foreground(colorCodeComment).Render(commentPart)
	}

	return codeStyle.Render(renderedCode)
}

func applyCodePattern(line string, pattern *regexp.Regexp, style lipgloss.Style) string {
	indexes := pattern.FindAllStringIndex(line, -1)
	if len(indexes) == 0 {
		return line
	}

	var out strings.Builder
	last := 0
	for _, loc := range indexes {
		start, end := loc[0], loc[1]
		if start < last {
			continue
		}
		out.WriteString(line[last:start])
		out.WriteString(style.Render(line[start:end]))
		last = end
	}
	out.WriteString(line[last:])
	return out.String()
}
