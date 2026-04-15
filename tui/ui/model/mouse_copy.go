package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
