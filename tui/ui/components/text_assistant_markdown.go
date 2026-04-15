package tui

import "strings"

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
		if url != "" && !isImage {
			b.WriteString(" (")
			b.WriteString(url)
			b.WriteString(")")
		}
		i = urlEnd + 1
	}
	return b.String()
}
