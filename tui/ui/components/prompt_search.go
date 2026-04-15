package tui

import (
	"fmt"
	"strings"

	"bytemind/internal/history"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handlePromptSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isPageUpKey(msg):
		m.stepPromptSearch(-promptSearchPageSize)
		return m, nil
	case isPageDownKey(msg):
		m.stepPromptSearch(promptSearchPageSize)
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closePromptSearch(true)
		m.statusNote = "Prompt search canceled."
		return m, nil
	case "enter":
		selected, ok := m.selectedPromptSearchEntry()
		if ok {
			m.setInputValue(selected.Prompt)
			m.closePromptSearch(false)
			m.statusNote = "Prompt restored from history."
			return m, nil
		}
		m.closePromptSearch(true)
		m.statusNote = "No prompt selected."
		return m, nil
	case "ctrl+f", "down", "j":
		m.stepPromptSearch(1)
		return m, nil
	case "ctrl+s", "up", "k":
		m.stepPromptSearch(-1)
		return m, nil
	case "home":
		m.stepPromptSearch(-len(m.promptSearchMatches))
		return m, nil
	case "end":
		m.stepPromptSearch(len(m.promptSearchMatches))
		return m, nil
	case "backspace", "ctrl+h":
		m.trimPromptSearchQuery()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyBackspace:
		m.trimPromptSearchQuery()
		return m, nil
	case tea.KeySpace:
		m.promptSearchQuery += " "
		m.refreshPromptSearchMatches()
		return m, nil
	case tea.KeyRunes:
		m.promptSearchQuery += string(msg.Runes)
		m.refreshPromptSearchMatches()
		return m, nil
	default:
		return m, nil
	}
}

func (m *model) openPromptSearch(mode promptSearchMode) {
	m.ensurePromptHistoryLoaded()
	m.promptSearchMode = mode
	m.promptSearchBaseInput = m.input.Value()
	m.promptSearchQuery = ""
	m.promptSearchCursor = 0
	m.promptSearchOpen = true
	m.commandOpen = false
	m.closeMentionPalette()
	m.refreshPromptSearchMatches()
	if len(m.promptSearchMatches) == 0 {
		if mode == promptSearchModePanel {
			m.statusNote = "History panel opened. No matching prompts."
		} else {
			m.statusNote = "No matching prompts."
		}
	} else {
		if mode == promptSearchModePanel {
			m.statusNote = fmt.Sprintf("History panel ready (%d matches).", len(m.promptSearchMatches))
		} else {
			m.statusNote = fmt.Sprintf("Prompt history ready (%d matches).", len(m.promptSearchMatches))
		}
	}
}

func (m *model) closePromptSearch(restoreInput bool) {
	if restoreInput {
		m.setInputValue(m.promptSearchBaseInput)
	}
	m.promptSearchOpen = false
	m.promptSearchMode = ""
	m.promptSearchQuery = ""
	m.promptSearchMatches = nil
	m.promptSearchCursor = 0
	m.promptSearchBaseInput = ""
	m.syncInputOverlays()
}

func (m *model) ensurePromptHistoryLoaded() {
	if m.promptHistoryLoaded {
		return
	}
	entries, err := m.runtimeAPI().LoadRecentPrompts(promptSearchLoadLimit)
	if err != nil {
		m.promptHistoryEntries = nil
		m.promptHistoryLoaded = true
		m.statusNote = "Prompt history unavailable: " + compact(err.Error(), 72)
		return
	}
	m.promptHistoryEntries = entries
	m.promptHistoryLoaded = true
}

func (m *model) refreshPromptSearchMatches() {
	tokens, workspaceFilter, sessionFilter := parsePromptSearchQuery(m.promptSearchQuery)
	limit := promptSearchResultCap
	if m.promptSearchMode == promptSearchModePanel {
		limit = promptSearchLoadLimit
	}
	matches := make([]history.PromptEntry, 0, min(len(m.promptHistoryEntries), limit))
	for i := len(m.promptHistoryEntries) - 1; i >= 0; i-- {
		entry := m.promptHistoryEntries[i]
		prompt := strings.TrimSpace(entry.Prompt)
		if prompt == "" {
			continue
		}
		workspaceValue := strings.ToLower(strings.TrimSpace(entry.Workspace))
		if workspaceFilter != "" && !strings.Contains(workspaceValue, workspaceFilter) {
			continue
		}
		sessionValue := strings.ToLower(strings.TrimSpace(entry.SessionID))
		if sessionFilter != "" && !strings.Contains(sessionValue, sessionFilter) {
			continue
		}
		promptLower := strings.ToLower(prompt)
		if !matchAllTokens(promptLower, tokens) {
			continue
		}
		matches = append(matches, entry)
		if len(matches) >= limit {
			break
		}
	}

	m.promptSearchMatches = matches
	if len(matches) == 0 {
		m.promptSearchCursor = 0
		return
	}
	m.promptSearchCursor = clamp(m.promptSearchCursor, 0, len(matches)-1)
}

func (m *model) stepPromptSearch(delta int) {
	if len(m.promptSearchMatches) == 0 {
		return
	}
	next := m.promptSearchCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.promptSearchMatches) {
		next = len(m.promptSearchMatches) - 1
	}
	m.promptSearchCursor = next
}

func (m *model) trimPromptSearchQuery() {
	if m.promptSearchQuery == "" {
		return
	}
	runes := []rune(m.promptSearchQuery)
	m.promptSearchQuery = string(runes[:len(runes)-1])
	m.refreshPromptSearchMatches()
}

func (m model) selectedPromptSearchEntry() (history.PromptEntry, bool) {
	if len(m.promptSearchMatches) == 0 {
		return history.PromptEntry{}, false
	}
	index := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	return m.promptSearchMatches[index], true
}

func (m model) visiblePromptSearchEntriesPage() []history.PromptEntry {
	if len(m.promptSearchMatches) == 0 {
		return nil
	}
	cursor := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	start := (cursor / promptSearchPageSize) * promptSearchPageSize
	end := min(len(m.promptSearchMatches), start+promptSearchPageSize)
	return m.promptSearchMatches[start:end]
}
