package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

var clipboardWriteTimeout = 2 * time.Second

// handleMouse routes mouse events between selection, scrollbar, plan panel and viewport.
func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ensureZoneManager()
	msg = m.normalizeMouseMsg(msg)

	if next, cmd, handled := m.handleInputMouseEvent(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleViewportMouseEvent(msg); handled {
		return next, cmd
	}

	if msg.Action == tea.MouseActionRelease {
		m.draggingScrollbar = false
	}
	if m.shouldIgnoreMouseEvent() {
		return m, nil
	}
	if cmd, consumed := m.tokenUsage.Update(msg); consumed {
		return m, cmd
	}
	if m.mouseOverInput(msg.Y) {
		return m.handleMouseOverInput(msg)
	}
	if m.screen != screenChat {
		return m, nil
	}
	if next, cmd, handled := m.handleChatScrollbarAndSelectionStart(msg); handled {
		return next, cmd
	}
	return m.handleChatViewportOrPlan(msg)
}

func (m model) shouldIgnoreMouseEvent() bool {
	if m.helpOpen || m.commandOpen || m.mentionOpen || m.promptSearchOpen || m.approval != nil {
		return true
	}
	if m.screen != screenChat && m.screen != screenLanding {
		return true
	}
	return m.screen == screenChat && m.sessionsOpen
}

func (m model) handleInputMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !m.inputMouseSelecting {
		return m, nil, false
	}
	switch msg.Action {
	case tea.MouseActionMotion:
		if point, ok := m.inputPointFromMouse(msg.X, msg.Y, true); ok {
			m.inputSelectionEnd = point
		} else {
			m.clearInputSelection()
		}
		return m, nil, true
	case tea.MouseActionRelease:
		if point, ok := m.inputPointFromMouse(msg.X, msg.Y, true); ok && selectionHasRange(m.inputSelectionStart, point) {
			m.inputSelectionEnd = point
			m.inputSelectionActive = true
			m.statusNote = "Selection ready. Press Ctrl+C to copy."
		} else {
			m.clearInputSelection()
		}
		m.inputMouseSelecting = false
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handleViewportMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !m.mouseSelecting {
		return m, nil, false
	}
	m.mouseSelectionMouseX = msg.X
	m.mouseSelectionMouseY = msg.Y
	switch msg.Action {
	case tea.MouseActionMotion:
		if point, ok := m.viewportPointFromMouseWithAutoScroll(msg.X, msg.Y); ok {
			m.mouseSelectionEnd = point
		} else {
			m.clearMouseSelection()
		}
		return m, nil, true
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
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handleMouseOverInput(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if m.mouseSelecting || m.mouseSelectionActive {
			m.clearMouseSelection()
		}
		if point, ok := m.inputPointFromMouse(msg.X, msg.Y, false); ok {
			m.inputMouseSelecting = true
			m.inputSelectionActive = false
			m.inputSelectionStart = point
			m.inputSelectionEnd = point
			return m, nil
		}
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

func (m model) handleChatScrollbarAndSelectionStart(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if next, cmd, handled := m.handleScrollbarDrag(msg); handled {
		return next, cmd, true
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil, false
	}
	if next, cmd, handled := m.handleScrollbarPress(msg); handled {
		return next, cmd, true
	}
	if m.mouseSelectionActive {
		m.clearMouseSelection()
	}
	if m.inputSelectionActive || m.inputMouseSelecting {
		m.clearInputSelection()
	}
	if point, ok := m.viewportPointFromMouse(msg.X, msg.Y); ok {
		m.mouseSelecting = true
		m.mouseSelectionActive = false
		m.mouseSelectionMouseX = msg.X
		m.mouseSelectionMouseY = msg.Y
		m.mouseSelectionStart = point
		m.mouseSelectionEnd = point
		return m, m.startMouseSelectionScrollTicker(), true
	}
	return m, nil, false
}

func (m model) handleScrollbarDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if msg.Action != tea.MouseActionMotion || !m.draggingScrollbar {
		return m, nil, false
	}
	m.dragScrollbarTo(msg.Y)
	m.chatAutoFollow = false
	return m, nil, true
}

func (m model) handleScrollbarPress(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	trackX, trackTop, trackBottom, ok := m.scrollbarTrackBounds()
	if !ok || msg.X != trackX || msg.Y < trackTop || msg.Y > trackBottom {
		return m, nil, false
	}
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || thumbHeight <= 0 {
		return m, nil, false
	}
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
	return m, nil, true
}

func (m model) handleChatViewportOrPlan(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleWheelScroll(msg); handled {
		return next, cmd
	}
	if m.mouseOverPlan(msg.X, msg.Y) {
		m.ensurePlanMouse()
		var cmd tea.Cmd
		m.planView, cmd = m.planView.Update(msg)
		return m, cmd
	}
	m.ensureViewportMouse()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	m.syncCopyViewOffset()
	m.chatAutoFollow = m.viewport.AtBottom()
	return m, cmd
}

func (m model) handleWheelScroll(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil, false
	}
	if m.mouseOverPlan(msg.X, msg.Y) {
		m.ensurePlanMouse()
		if msg.Button == tea.MouseButtonWheelUp {
			m.planView.LineUp(scrollStep)
		} else {
			m.planView.LineDown(scrollStep)
		}
		return m, nil, true
	}
	m.ensureViewportMouse()
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.viewport.LineUp(scrollStep)
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
	default:
		m.viewport.LineDown(scrollStep)
		m.syncCopyViewOffset()
		m.chatAutoFollow = m.viewport.AtBottom()
	}
	return m, nil, true
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
	selected := m.inputSelectionText()
	if strings.TrimSpace(selected) == "" {
		selected = m.viewportSelectionText()
	}
	if strings.TrimSpace(selected) != "" {
		if m.clipboardText == nil {
			m.statusNote = "clipboard copy is unavailable in current environment"
		} else if err := m.writeSelectionToClipboard(selected); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				m.statusNote = "clipboard copy timed out"
				return nil
			}
			m.statusNote = err.Error()
		} else {
			m.statusNote = "Copied selection to clipboard."
			m.selectionToast = "Copied selection"
			m.selectionToastID++
			id := m.selectionToastID
			m.clearMouseSelection()
			m.clearInputSelection()
			return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return selectionToastExpiredMsg{ID: id}
			})
		}
		return nil
	}
	m.statusNote = "Selection is empty."
	m.clearMouseSelection()
	m.clearInputSelection()
	return nil
}

func (m *model) writeSelectionToClipboard(selected string) error {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardWriteTimeout)
	defer cancel()
	return m.clipboardText.WriteText(ctx, selected)
}

func (m model) normalizeMouseMsg(msg tea.MouseMsg) tea.MouseMsg {
	if m.mouseYOffset == 0 {
		return msg
	}
	if m.screen == screenLanding {
		// Landing input uses zone-based hit testing; applying an extra global
		// y-offset here introduces row drift in some Windows terminals.
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

func (m *model) clearInputSelection() {
	if m == nil {
		return
	}
	m.inputMouseSelecting = false
	m.inputSelectionActive = false
	m.inputSelectionStart = viewportSelectionPoint{}
	m.inputSelectionEnd = viewportSelectionPoint{}
}

func (m model) hasCopyableSelection() bool {
	return m.hasCopyableInputSelection() || m.hasCopyableViewportSelection()
}

func (m model) hasCopyableInputSelection() bool {
	return (m.inputMouseSelecting || m.inputSelectionActive) && selectionHasRange(m.inputSelectionStart, m.inputSelectionEnd)
}

func (m model) hasCopyableViewportSelection() bool {
	return (m.mouseSelecting || m.mouseSelectionActive) && selectionHasRange(m.mouseSelectionStart, m.mouseSelectionEnd)
}

func (m model) renderConversationViewport() string {
	return m.conversationViewportComponent().Render(m)
}

func renderConversationViewportDefault(m model) string {
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
	return m.inputEditorViewComponent().Render(m)
}

func renderInputEditorViewDefault(m model) string {
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

// viewportPointFromMouse maps terminal mouse coordinates into absolute viewport
// selection coordinates. It prefers bubblezone data first, then falls back to
// layout-derived bounds for terminals with imperfect mouse reports. Zone lookup
// auto-probes up to +/-4 rows to recover from terminal row drift.
func (m *model) viewportPointFromMouse(x, y int) (viewportSelectionPoint, bool) {
	if m == nil {
		return viewportSelectionPoint{}, false
	}
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

// inputPointFromMouse converts mouse coordinates to logical input-editor cells.
// When clampToBounds is true, out-of-bound points are clamped to the nearest edge.
// Zone lookup auto-probes up to +/-4 rows to recover from terminal row drift.
func (m model) inputPointFromMouse(x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	ensureZoneManager()
	if z := zone.Get(inputEditorZoneID); z != nil {
		if point, ok := m.inputPointFromZone(z, x, y, clampToBounds); ok {
			return point, true
		}
		// Keep input selection robust for terminals that occasionally
		// report mouse rows with small absolute drift.
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.inputPointFromZone(z, x, y-delta, clampToBounds); ok {
					return point, true
				}
				if point, ok := m.inputPointFromZone(z, x, y+delta, clampToBounds); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, innerLeft, innerTop, ok := m.inputInnerBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if !clampToBounds && (x < left || x > right || y < top || y > bottom) {
		return viewportSelectionPoint{}, false
	}
	x = clamp(x, left, right)
	y = clamp(y, top, bottom)
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row := clamp(y-innerTop, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col := clamp(x-innerLeft, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputPointFromZone(z *zone.ZoneInfo, x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if (col < 0 || row < 0) && clampToBounds {
		clampedX := clamp(x, z.StartX, z.EndX)
		clampedY := clamp(y, z.StartY, z.EndY)
		col, row = z.Pos(tea.MouseMsg{X: clampedX, Y: clampedY})
	}
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row = clamp(row, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col = clamp(col, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputInnerBounds() (left, right, top, bottom, innerLeft, innerTop int, ok bool) {
	switch m.screen {
	case screenChat:
		if m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		inputRender := m.inputBorderStyle().Width(m.chatPanelInnerWidth()).Render(m.input.View())
		left = panelStyle.GetHorizontalFrameSize() / 2
		top = panelStyle.GetVerticalFrameSize()/2 + lipgloss.Height(m.renderMainPanel())
		if m.approval != nil {
			top += lipgloss.Height(m.renderApprovalBanner())
		}
		top += m.calculateOverlayHeight(0)
		right = left + max(1, lipgloss.Width(inputRender)) - 1
		bottom = top + max(1, lipgloss.Height(inputRender)) - 1
		innerLeft = left + m.inputBorderStyle().GetHorizontalFrameSize()/2
		innerTop = top + m.inputBorderStyle().GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	case screenLanding:
		if m.height <= 0 || m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		box := landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View())
		boxW := max(1, lipgloss.Width(box))
		boxH := max(1, lipgloss.Height(box))
		logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
			"    ____        __                      _           __",
			"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
			"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
			" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
			"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
			"      /____/                                          ",
		}, "\n")))
		overlayHeight := m.calculateOverlayHeight(1)
		modeTabsHeight := lipgloss.Height(m.renderModeTabs())
		hintHeight := lipgloss.Height(mutedStyle.Render(footerHintText))
		contentHeight := logoHeight + 1 + modeTabsHeight + 1 + overlayHeight + boxH + 1 + hintHeight
		contentTop := max(0, (m.height-contentHeight)/2)
		top = contentTop + logoHeight + 1 + modeTabsHeight + 1 + overlayHeight
		left = max(0, (m.width-boxW)/2)
		right = left + boxW - 1
		bottom = top + boxH - 1
		innerLeft = left + landingInputStyle.GetHorizontalFrameSize()/2
		innerTop = top + landingInputStyle.GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	default:
		return 0, 0, 0, 0, 0, 0, false
	}
}

func (m model) calculateOverlayHeight(extraGap int) int {
	switch {
	case m.startupGuide.Active:
		return lipgloss.Height(m.renderStartupGuidePanel()) + extraGap
	case m.promptSearchOpen:
		return lipgloss.Height(m.renderPromptSearchPalette()) + extraGap
	case m.mentionOpen:
		return lipgloss.Height(m.renderMentionPalette()) + extraGap
	case m.commandOpen:
		return lipgloss.Height(m.renderCommandPalette()) + extraGap
	default:
		return 0
	}
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
