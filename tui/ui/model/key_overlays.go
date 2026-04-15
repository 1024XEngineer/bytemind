package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleQuitKey(msg); handled {
		return next, cmd
	}
	if m.promptSearchOpen {
		return m.handlePromptSearchKey(msg)
	}
	if next, cmd, handled := m.handleGlobalShortcutKey(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleOverlayKey(msg); handled {
		return next, cmd
	}
	return m.handleComposerKey(msg)
}

func (m model) handleQuitKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "ctrl+c" {
		return m, nil, false
	}
	if m.hasCopyableSelection() {
		return m, m.copyCurrentSelection(), true
	}
	if m.approval != nil {
		m.approval.Reply <- approvalDecision{Approved: false}
	}
	if m.runCancel != nil {
		m.runCancel()
	}
	return m, tea.Quit, true
}

func (m model) handleGlobalShortcutKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		if m.hasCopyableSelection() {
			m.clearMouseSelection()
			m.clearInputSelection()
			m.statusNote = "Selection cleared."
			return m, nil, true
		}
	case "tab":
		if m.commandOpen || m.mentionOpen || m.sessionsOpen || m.helpOpen || m.approval != nil {
			return m, nil, false
		}
		m.toggleMode()
		return m, nil, true
	case "ctrl+g":
		if m.approval == nil {
			m.helpOpen = !m.helpOpen
		}
		return m, nil, true
	case "ctrl+f":
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.skillsOpen || m.commandOpen || m.mentionOpen {
			return m, nil, true
		}
		m.openPromptSearch(promptSearchModeQuick)
		return m, nil, true
	case "ctrl+k":
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.commandOpen || m.mentionOpen || m.busy {
			return m, nil, true
		}
		if err := m.openSkillsPicker(); err != nil {
			m.statusNote = err.Error()
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	return m, nil, false
}

func (m model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.approval != nil {
		return m.handleApprovalKey(msg)
	}
	if m.helpOpen {
		return m.handleHelpKey(msg)
	}
	if m.commandOpen {
		next, cmd := m.handleCommandPaletteKey(msg)
		return next, cmd, true
	}
	if m.skillsOpen {
		return m.handleSkillsKey(msg)
	}
	if m.mentionOpen {
		next, cmd := m.handleMentionPaletteKey(msg)
		return next, cmd, true
	}
	if m.sessionsOpen {
		return m.handleSessionsKey(msg)
	}
	return m, nil, false
}

func (m model) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.approval.Reply <- approvalDecision{Approved: true}
		m.statusNote = "Shell command approved."
		m.phase = "tool"
		m.approval = nil
	case "n", "N", "esc":
		m.approval.Reply <- approvalDecision{Approved: false}
		m.statusNote = "Shell command rejected."
		m.phase = "thinking"
		m.approval = nil
	}
	return m, nil, true
}

func (m model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() == "esc" || msg.String() == "ctrl+g" {
		m.helpOpen = false
	}
	return m, nil, true
}

func (m model) handleSkillsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	items := m.skillPickerItems()
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-commandPageSize)
		}
		return m, nil, true
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+commandPageSize)
		}
		return m, nil, true
	}

	switch msg.String() {
	case "esc":
		m.skillsOpen = false
		m.commandCursor = 0
	case "up", "k":
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-1)
		}
	case "down", "j":
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+1)
		}
	case "enter":
		if err := m.activateSelectedSkill(); err != nil {
			m.statusNote = err.Error()
			return m, nil, true
		}
		m.skillsOpen = false
		m.commandCursor = 0
	}
	return m, nil, true
}

func (m model) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		m.sessionsOpen = false
	case "up", "k":
		if m.sessionCursor > 0 {
			m.sessionCursor--
		}
	case "down", "j":
		if m.sessionCursor < len(m.sessions)-1 {
			m.sessionCursor++
		}
	case "enter":
		if m.busy || len(m.sessions) == 0 {
			return m, nil, true
		}
		if err := m.resumeSession(m.sessions[m.sessionCursor].ID); err != nil {
			m.statusNote = err.Error()
		} else {
			m.sessionsOpen = false
		}
	}
	return m, nil, true
}
