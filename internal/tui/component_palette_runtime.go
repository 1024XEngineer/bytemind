package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"bytemind/internal/history"
	"bytemind/internal/mention"
)

func (m *model) syncCommandPalette() {
	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		m.commandOpen = false
		m.commandCursor = 0
		return
	}
	m.commandOpen = true
	m.closeMentionPalette()
	items := m.filteredCommands()
	if len(items) == 0 {
		m.commandCursor = 0
		return
	}
	if m.commandCursor < 0 || m.commandCursor >= len(items) {
		m.commandCursor = 0
	}
}

func (m *model) syncInputOverlays() {
	if m.startupGuide.Active || m.promptSearchOpen {
		return
	}
	m.syncCommandPalette()
	m.syncMentionPalette()
	m.syncInputImageRefs(m.input.Value())
}

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

func (m *model) closeMentionPalette() {
	m.mentionOpen = false
	m.mentionCursor = 0
	m.mentionQuery = ""
	m.mentionToken = mention.Token{}
	m.mentionResults = nil
}

func (m *model) recordRecentMention(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if m.mentionRecent == nil {
		m.mentionRecent = make(map[string]int, 16)
	}
	m.mentionSeq++
	m.mentionRecent[path] = m.mentionSeq
}

func (m model) hasRecentMention(path string) bool {
	if m.mentionRecent == nil {
		return false
	}
	return m.mentionRecent[path] > 0
}

func (m model) filteredCommands() []commandItem {
	value := strings.TrimSpace(m.input.Value())
	query := commandFilterQuery(value, "")
	items := m.commandPaletteItems()
	if query == "" {
		return items
	}

	result := make([]commandItem, 0, len(items))
	for _, item := range items {
		if matchesCommandItem(item, query) {
			result = append(result, item)
		}
	}
	return result
}

func (m model) commandPaletteItems() []commandItem {
	items := visibleCommandItems("")
	skills := m.skillPickerItems()
	if len(skills) == 0 {
		return items
	}
	merged := make([]commandItem, 0, len(items)+len(skills))
	merged = append(merged, items...)
	merged = append(merged, skills...)
	return merged
}

func (m model) skillPickerItems() []commandItem {
	if m.runner == nil {
		return nil
	}
	skillsList, _ := m.runner.ListSkills()
	if len(skillsList) == 0 {
		return nil
	}

	items := make([]commandItem, 0, len(skillsList))
	seen := make(map[string]struct{}, len(skillsList))
	for _, skill := range skillsList {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = fmt.Sprintf("Activate %s for this session.", skill.Name)
		}
		items = append(items, commandItem{
			Name:        name,
			Usage:       "/" + name,
			Description: description,
			Kind:        "skill",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Usage < items[j].Usage
	})
	return items
}

func (m model) commandPaletteWidth() int {
	switch m.screen {
	case screenLanding:
		return max(28, m.landingInputShellWidth())
	default:
		return max(32, m.chatPanelInnerWidth())
	}
}

func (m model) visibleCommandItemsPage() []commandItem {
	items := m.filteredCommands()
	if len(items) == 0 {
		return nil
	}
	cursor := clamp(m.commandCursor, 0, len(items)-1)
	start := (cursor / commandPageSize) * commandPageSize
	end := min(len(items), start+commandPageSize)
	return items[start:end]
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

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}
