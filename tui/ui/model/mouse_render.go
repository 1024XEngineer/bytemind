package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

func (m model) renderConversationViewport() string {
	content := ""
	if m.hasCopyableViewportSelection() {
		if preview := m.renderActiveSelectionPreview(); strings.TrimSpace(preview) != "" {
			content = preview
		}
	}
	if content == "" {
		content = m.viewport.View()
	}
	return zone.Mark(conversationViewportZoneID, content)
}

func (m model) renderInputEditorView() string {
	raw := m.input.View()
	if !m.hasCopyableInputSelection() {
		return raw
	}
	if preview := m.renderInputSelectionPreview(raw); preview != "" {
		return preview
	}
	return raw
}

func (m model) renderInputSelectionPreview(raw string) string {
	lines := m.inputSelectionSourceLines(raw)
	return renderSelectionPreviewLines(lines, m.inputSelectionStart, m.inputSelectionEnd, 0, 1, len(lines)-1, raw)
}

func (m model) renderActiveSelectionPreview() string {
	view := strings.ReplaceAll(m.viewport.View(), "\r\n", "\n")
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return ""
	}

	sourceLines := m.selectionSourceLines()
	return renderSelectionPreviewLines(lines, m.mouseSelectionStart, m.mouseSelectionEnd, max(0, m.viewport.YOffset), m.viewport.Width, len(sourceLines)-1, "")
}

func (m model) viewportSelectionText() string {
	lines := m.selectionSourceLines()
	return selectionTextFromLines(lines, m.mouseSelectionStart, m.mouseSelectionEnd, m.viewport.Width)
}

func (m model) inputSelectionText() string {
	if strings.TrimSpace(m.input.Value()) == "" {
		return ""
	}
	lines := m.inputSelectionSourceLines("")
	return selectionTextFromLines(lines, m.inputSelectionStart, m.inputSelectionEnd, 1)
}

func (m model) selectionSourceLines() []string {
	content := m.viewportContentCache
	if content == "" {
		content = m.viewport.View()
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(content, "\n")
}

func (m model) inputSelectionSourceLines(raw string) []string {
	if raw == "" {
		raw = m.input.View()
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	return strings.Split(raw, "\n")
}

func renderSelectionPreviewLines(
	lines []string,
	start viewportSelectionPoint,
	end viewportSelectionPoint,
	rowBase int,
	minLineWidth int,
	maxSelectionRow int,
	fallback string,
) string {
	start, end, ok := normalizeSelectionForRowLimit(start, end, maxSelectionRow)
	if !ok {
		return fallback
	}
	rendered := make([]string, 0, len(lines))
	for row, line := range lines {
		lineWidth := max(minLineWidth, xansi.StringWidth(line))
		selectionRow := rowBase + row
		rangeStart, rangeEnd, inRange := selectionColumnsForRow(selectionRow, lineWidth, start, end, false)
		if !inRange {
			rendered = append(rendered, line)
			continue
		}
		rendered = append(rendered, highlightVisibleLineByCells(line, rangeStart, rangeEnd))
	}
	if len(rendered) == 0 {
		return fallback
	}
	return strings.Join(rendered, "\n")
}

func selectionTextFromLines(lines []string, start, end viewportSelectionPoint, minLineWidth int) string {
	start, end, ok := normalizeSelectionForRowLimit(start, end, len(lines)-1)
	if !ok {
		return ""
	}
	parts := make([]string, 0, end.Row-start.Row+1)
	for row := start.Row; row <= end.Row; row++ {
		raw := lines[row]
		lineWidth := max(minLineWidth, xansi.StringWidth(raw))
		rangeStart, rangeEnd, inRange := selectionColumnsForRow(row, lineWidth, start, end, false)
		if !inRange {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, sliceViewportLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(parts, "\n")
}

func normalizeSelectionForRowLimit(start, end viewportSelectionPoint, maxRow int) (viewportSelectionPoint, viewportSelectionPoint, bool) {
	if maxRow < 0 {
		return viewportSelectionPoint{}, viewportSelectionPoint{}, false
	}
	start, end = normalizeViewportSelectionPoints(start, end)
	if start.Row == end.Row && start.Col == end.Col {
		return viewportSelectionPoint{}, viewportSelectionPoint{}, false
	}
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)
	if start.Row > end.Row {
		start, end = end, start
	}
	return start, end, true
}

func sliceViewportLineByCells(line string, startCol, endCol int) string {
	width := xansi.StringWidth(line)
	if width == 0 {
		return ""
	}
	if endCol < startCol {
		return ""
	}
	start := clamp(startCol, 0, width-1)
	end := clamp(endCol+1, start+1, width)
	return strings.TrimRight(xansi.Strip(xansi.Cut(line, start, end)), " ")
}

func normalizeViewportSelectionPoints(start, end viewportSelectionPoint) (viewportSelectionPoint, viewportSelectionPoint) {
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		return end, start
	}
	return start, end
}

func selectionColumnsForRow(row, width int, start, end viewportSelectionPoint, includePoint bool) (int, int, bool) {
	if width <= 0 || row < start.Row || row > end.Row {
		return 0, 0, false
	}
	if start.Row == end.Row {
		if start.Col == end.Col {
			if includePoint && row == start.Row {
				col := clamp(start.Col, 0, width-1)
				return col, col, true
			}
			return 0, 0, false
		}
		return clamp(start.Col, 0, width-1), clamp(end.Col, 0, width-1), true
	}
	switch row {
	case start.Row:
		return clamp(start.Col, 0, width-1), width - 1, true
	case end.Row:
		return 0, clamp(end.Col, 0, width-1), true
	default:
		return 0, width - 1, true
	}
}

func highlightVisibleLineByCells(line string, startCol, endCol int) string {
	width := xansi.StringWidth(line)
	if width == 0 {
		return ""
	}
	if endCol < startCol {
		return line
	}
	start := clamp(startCol, 0, width-1)
	end := clamp(endCol+1, start+1, width)
	left := xansi.Cut(line, 0, start)
	middle := selectionHighlightStyle.Render(xansi.Strip(xansi.Cut(line, start, end)))
	right := xansi.Cut(line, end, width)
	return left + middle + right
}

func selectionHasRange(start, end viewportSelectionPoint) bool {
	return start.Row != end.Row || start.Col != end.Col
}
