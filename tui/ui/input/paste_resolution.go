package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

func countPastedDisplayLines(content string) int {
	normalized := normalizeNewlines(content)
	physicalLines := strings.Split(normalized, "\n")
	if len(physicalLines) > 1 {
		return len(physicalLines)
	}

	single := strings.TrimSpace(normalized)
	if single == "" {
		return 1
	}
	width := runewidth.StringWidth(single)
	if width <= 0 {
		return 1
	}
	estimated := (width + pasteSoftWrapWidth - 1) / pasteSoftWrapWidth
	if estimated < 1 {
		return 1
	}
	return estimated
}
