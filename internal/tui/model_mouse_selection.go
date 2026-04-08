package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ensureZoneManager()
	msg = m.normalizeMouseMsg(msg)

	if m.mouseSelecting {
		m.mouseSelectionMouseX = msg.X
		m.mouseSelectionMouseY = msg.Y
		switch msg.Action {
		case tea.MouseActionMotion:
			if point, ok := m.viewportPointFromMouseWithAutoScroll(msg.X, msg.Y); ok {
				m.mouseSelectionEnd = point
			} else {
				m.clearMouseSelection()
			}
			return m, nil
		case tea.MouseActionRelease:
			if point, ok := m.viewportPointFromMouseWithAutoScroll(msg.X, msg.Y); ok && selectionHasRange(m.mouseSelectionStart, point) {
				m.mouseSelectionEnd = point
				m.mouseSelectionActive = true
				m.statusNote = "Selection ready. Press Ctrl+C to copy."
			} else {
				m.clearMouseSelection()
			}
			m.mouseSelecting = false
			m.stopMouseSelectionScrollTicker()
			m.draggingScrollbar = false
			return m, nil
		}
	}

	if msg.Action == tea.MouseActionRelease {
		m.draggingScrollbar = false
	}
	if m.helpOpen || m.commandOpen || m.mentionOpen || m.promptSearchOpen || m.approval != nil {
		return m, nil
	}
	if m.screen != screenChat && m.screen != screenLanding {
		return m, nil
	}
	if m.screen == screenChat && m.sessionsOpen {
		return m, nil
	}
	if cmd, consumed := m.tokenUsage.Update(msg); consumed {
		return m, cmd
	}
	if m.mouseOverInput(msg.Y) {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && (m.mouseSelecting || m.mouseSelectionActive) {
			m.clearMouseSelection()
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollInput(-scrollStep)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollInput(scrollStep)
			return m, nil
		default:
			return m, nil
		}
	}
	if m.screen == screenChat {
		if msg.Action == tea.MouseActionMotion && m.draggingScrollbar {
			m.dragScrollbarTo(msg.Y)
			m.chatAutoFollow = false
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			trackX, trackTop, trackBottom, ok := m.scrollbarTrackBounds()
			if ok && msg.X == trackX && msg.Y >= trackTop && msg.Y <= trackBottom {
				thumbTop, thumbHeight, _, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
				if visible && thumbHeight > 0 {
					absoluteThumbTop := trackTop + thumbTop
					absoluteThumbBottom := absoluteThumbTop + thumbHeight - 1
					if msg.Y >= absoluteThumbTop && msg.Y <= absoluteThumbBottom {
						m.scrollbarDragOffset = msg.Y - absoluteThumbTop
					} else {
						// Click on track should jump close to that point, then start drag.
						m.scrollbarDragOffset = thumbHeight / 2
						m.dragScrollbarTo(msg.Y)
					}
					m.draggingScrollbar = true
					m.chatAutoFollow = false
					return m, nil
				}
			}
			if m.mouseSelectionActive {
				m.clearMouseSelection()
			}
			if point, ok := m.viewportPointFromMouse(msg.X, msg.Y); ok {
				m.mouseSelecting = true
				m.mouseSelectionActive = false
				m.mouseSelectionMouseX = msg.X
				m.mouseSelectionMouseY = msg.Y
				m.mouseSelectionStart = point
				m.mouseSelectionEnd = point
				return m, m.startMouseSelectionScrollTicker()
			}
		}
	}
	if m.screen == screenChat {
		if m.mouseOverPlan(msg.X, msg.Y) {
			m.ensurePlanMouse()
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.planView.LineUp(scrollStep)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.planView.LineDown(scrollStep)
				return m, nil
			default:
				var cmd tea.Cmd
				m.planView, cmd = m.planView.Update(msg)
				return m, cmd
			}
		}
		m.ensureViewportMouse()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(scrollStep)
			m.syncCopyViewOffset()
			m.chatAutoFollow = false
			return m, nil
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(scrollStep)
			m.syncCopyViewOffset()
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.syncCopyViewOffset()
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, cmd
		}
	}
	return m, nil
}

func (m model) handleMouseSelectionScrollTick(msg mouseSelectionScrollTickMsg) (tea.Model, tea.Cmd) {
	if !m.mouseSelecting || msg.ID != m.mouseSelectionTickID {
		return m, nil
	}
	cmd := mouseSelectionScrollTickCmd(msg.ID)
	if !selectionHasRange(m.mouseSelectionStart, m.mouseSelectionEnd) {
		return m, cmd
	}
	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return m, cmd
	}
	if m.mouseSelectionMouseX < left-1 || m.mouseSelectionMouseX > right+1 {
		return m, cmd
	}

	targetY := 0
	switch {
	case m.mouseSelectionMouseY >= bottom:
		targetY = bottom + 1
	case m.mouseSelectionMouseY <= top:
		targetY = top - 1
	default:
		return m, cmd
	}
	if point, ok := m.viewportPointFromMouseWithAutoScroll(m.mouseSelectionMouseX, targetY); ok {
		m.mouseSelectionEnd = point
	}
	return m, cmd
}

func (m *model) startMouseSelectionScrollTicker() tea.Cmd {
	if m == nil {
		return nil
	}
	m.mouseSelectionTickID++
	return mouseSelectionScrollTickCmd(m.mouseSelectionTickID)
}

func (m *model) stopMouseSelectionScrollTicker() {
	if m == nil {
		return
	}
	m.mouseSelectionTickID++
}

func mouseSelectionScrollTickCmd(id int) tea.Cmd {
	return tea.Tick(mouseSelectionScrollTick, func(time.Time) tea.Msg {
		return mouseSelectionScrollTickMsg{ID: id}
	})
}

func (m *model) viewportPointFromMouseWithAutoScroll(x, y int) (viewportSelectionPoint, bool) {
	if m == nil {
		return viewportSelectionPoint{}, false
	}
	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if x < left-1 || x > right+1 {
		return viewportSelectionPoint{}, false
	}

	edgeX := clamp(x, left, right)
	switch {
	case y > bottom:
		steps := y - bottom
		for i := 0; i < steps; i++ {
			m.viewport.LineDown(1)
		}
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m.viewportPointFromMouse(edgeX, bottom)
	case y < top:
		steps := top - y
		for i := 0; i < steps; i++ {
			m.viewport.LineUp(1)
		}
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m.viewportPointFromMouse(edgeX, top)
	default:
		return m.viewportPointFromMouse(x, y)
	}
}

func (m *model) copyCurrentSelection() tea.Cmd {
	if m == nil {
		return nil
	}
	if selected := m.viewportSelectionText(); strings.TrimSpace(selected) != "" {
		if m.clipboardText == nil {
			m.statusNote = "clipboard copy is unavailable in current environment"
		} else if err := m.clipboardText.WriteText(context.Background(), selected); err != nil {
			m.statusNote = err.Error()
		} else {
			m.statusNote = "Copied selection to clipboard."
			m.selectionToast = "Copied selection"
			m.selectionToastID++
			id := m.selectionToastID
			m.clearMouseSelection()
			return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return selectionToastExpiredMsg{ID: id}
			})
		}
		return nil
	}
	m.statusNote = "Selection is empty."
	m.clearMouseSelection()
	return nil
}

func (m model) normalizeMouseMsg(msg tea.MouseMsg) tea.MouseMsg {
	if m.mouseYOffset == 0 {
		return msg
	}
	msg.Y += m.mouseYOffset
	return msg
}

func (m *model) clearMouseSelection() {
	if m == nil {
		return
	}
	m.stopMouseSelectionScrollTicker()
	m.mouseSelecting = false
	m.mouseSelectionActive = false
	m.mouseSelectionMouseX = 0
	m.mouseSelectionMouseY = 0
	m.mouseSelectionStart = viewportSelectionPoint{}
	m.mouseSelectionEnd = viewportSelectionPoint{}
}

func (m model) hasCopyableSelection() bool {
	return (m.mouseSelecting || m.mouseSelectionActive) && selectionHasRange(m.mouseSelectionStart, m.mouseSelectionEnd)
}

func (m model) renderConversationViewport() string {
	content := ""
	if m.hasCopyableSelection() {
		if preview := m.renderActiveSelectionPreview(); strings.TrimSpace(preview) != "" {
			content = preview
		}
	}
	if content == "" {
		content = m.viewport.View()
	}
	return zone.Mark(conversationViewportZoneID, content)
}

func (m model) renderActiveSelectionPreview() string {
	view := strings.ReplaceAll(m.viewport.View(), "\r\n", "\n")
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return ""
	}

	start, end := normalizeViewportSelectionPoints(m.mouseSelectionStart, m.mouseSelectionEnd)
	sourceLines := m.selectionSourceLines()
	maxRow := len(sourceLines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)

	rendered := make([]string, 0, len(lines))
	visibleStartRow := max(0, m.viewport.YOffset)
	for row, raw := range lines {
		lineWidth := max(xansi.StringWidth(raw), m.viewport.Width)
		absRow := visibleStartRow + row
		rangeStart, rangeEnd, ok := selectionColumnsForRow(absRow, lineWidth, start, end, false)
		if !ok {
			rendered = append(rendered, raw)
			continue
		}
		rendered = append(rendered, highlightVisibleLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(rendered, "\n")
}

func (m model) viewportPointFromMouse(x, y int) (viewportSelectionPoint, bool) {
	ensureZoneManager()
	if z := zone.Get(conversationViewportZoneID); z != nil {
		if point, ok := m.viewportPointFromZone(z, x, y); ok {
			return point, true
		}
		// Keep zone-first behavior robust for terminals that occasionally
		// report mouse rows with small absolute drift near viewport edges.
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.viewportPointFromZone(z, x, y-delta); ok {
					return point, true
				}
				if point, ok := m.viewportPointFromZone(z, x, y+delta); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if renderedTop, found := m.conversationViewportTopFromRenderedView(left, top); found {
		top = renderedTop
		bottom = top + m.viewport.Height - 1
	}
	// Keep drag-select usable for terminals that report 0-based or 1-based mouse coords.
	if x < left-1 || x > right+1 || y < top-1 || y > bottom+1 {
		return viewportSelectionPoint{}, false
	}
	col := clamp(x-left, 0, max(0, m.viewport.Width-1))
	row := clamp(y-top, 0, max(0, m.viewport.Height-1))
	return viewportSelectionPoint{
		Col: col,
		Row: max(0, m.viewport.YOffset) + row,
	}, true
}

func (m model) viewportPointFromZone(z *zone.ZoneInfo, x, y int) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	return viewportSelectionPoint{
		Col: clamp(col, 0, max(0, m.viewport.Width-1)),
		Row: max(0, m.viewport.YOffset) + clamp(row, 0, max(0, m.viewport.Height-1)),
	}, true
}

func (m model) viewportSelectionText() string {
	start, end := normalizeViewportSelectionPoints(m.mouseSelectionStart, m.mouseSelectionEnd)
	if start.Row == end.Row && start.Col == end.Col {
		return ""
	}

	lines := m.selectionSourceLines()
	if len(lines) == 0 {
		return ""
	}
	maxRow := len(lines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)
	if start.Row > end.Row {
		start, end = end, start
	}

	parts := make([]string, 0, end.Row-start.Row+1)
	for row := start.Row; row <= end.Row; row++ {
		raw := lines[row]
		lineWidth := max(xansi.StringWidth(raw), m.viewport.Width)
		rangeStart, rangeEnd, ok := selectionColumnsForRow(row, lineWidth, start, end, false)
		if !ok {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, sliceViewportLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(parts, "\n")
}

func (m model) selectionSourceLines() []string {
	content := m.viewportContentCache
	if content == "" {
		content = m.viewport.View()
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(content, "\n")
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
