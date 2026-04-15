package tui

import tea "github.com/charmbracelet/bubbletea"

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
