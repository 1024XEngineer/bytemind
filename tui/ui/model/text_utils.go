package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

func trimPreview(text string, limit int) string {
	return compact(text, limit)
}

func compact(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	if limit <= 0 || runewidth.StringWidth(text) <= limit {
		return text
	}
	if limit <= runewidth.StringWidth("...") {
		return runewidth.Truncate(text, limit, "")
	}
	return runewidth.Truncate(text, limit, "...")
}

func emptyDot(path string) string {
	if strings.TrimSpace(path) == "" {
		return "."
	}
	return path
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
