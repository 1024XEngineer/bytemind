package tui

import (
	"bytemind/internal/mention"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) syncMentionPalette() {
	if m.commandOpen {
		m.closeMentionPalette()
		return
	}
	token, ok := mention.FindActiveToken(m.input.Value())
	if !ok {
		m.closeMentionPalette()
		return
	}

	if m.mentionIndex == nil {
		m.mentionIndex = mention.NewWorkspaceFileIndex(m.workspace)
	}
	results := m.mentionIndex.SearchWithRecency(token.Query, mentionPageSize*3, m.mentionRecent)
	m.mentionOpen = true
	m.mentionQuery = token.Query
	m.mentionToken = token
	m.mentionResults = results

	if len(results) == 0 {
		m.mentionCursor = 0
		return
	}
	if m.mentionCursor < 0 || m.mentionCursor >= len(results) {
		m.mentionCursor = 0
	}
}

func (m model) handleMentionPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.mentionResults
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-mentionPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+mentionPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeMentionPalette()
		return m, nil
	case "up", "k":
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-1)
		}
		return m, nil
	case "down", "j":
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+1)
		}
		return m, nil
	case "tab":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			return m, nil
		}
		m.applyMentionSelection(selected)
		return m, nil
	case "enter":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			m.closeMentionPalette()
			return m.handleKey(msg)
		}
		m.applyMentionSelection(selected)
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m *model) closeMentionPalette() {
	m.mentionOpen = false
	m.mentionCursor = 0
	m.mentionQuery = ""
	m.mentionToken = mention.Token{}
	m.mentionResults = nil
}

func (m model) selectedMentionCandidate() (mention.Candidate, bool) {
	if len(m.mentionResults) == 0 {
		return mention.Candidate{}, false
	}
	index := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	return m.mentionResults[index], true
}

func (m model) visibleMentionItemsPage() []mention.Candidate {
	if len(m.mentionResults) == 0 {
		return nil
	}
	cursor := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	start := (cursor / mentionPageSize) * mentionPageSize
	end := min(len(m.mentionResults), start+mentionPageSize)
	return m.mentionResults[start:end]
}
