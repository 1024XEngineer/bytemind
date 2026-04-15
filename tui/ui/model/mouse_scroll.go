package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
		m.scrollbarDragOffset = thumbHeight / 2
		m.dragScrollbarTo(msg.Y)
	}
	m.draggingScrollbar = true
	m.chatAutoFollow = false
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
