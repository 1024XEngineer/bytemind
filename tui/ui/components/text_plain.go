package tui

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

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
