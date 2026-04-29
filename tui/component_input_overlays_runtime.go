package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/1024XEngineer/bytemind/internal/history"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/mention"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func (m model) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.filteredCommands()
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-commandPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+commandPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeCommandPalette()
		return m, nil
	case "up":
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-1)
		}
		return m, nil
	case "down":
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+1)
		}
		return m, nil
	case "enter":
		selected, ok := m.selectedCommandItem()
		if !ok {
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				return m, nil
			}
			if value == "/quit" {
				m.closeCommandPalette()
				return m, tea.Quit
			}
			if m.busy {
				if isBTWCommand(value) {
					btw, err := extractBTWText(value)
					if err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
					m.closeCommandPalette()
					return m.submitBTW(btw)
				}
				if strings.HasPrefix(value, "/") {
					m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
					return m, nil
				}
				m.closeCommandPalette()
				return m.submitBTW(value)
			}
			m.closeCommandPalette()
			m.input.Reset()
			m.clearPasteTransaction()
			m.clearVirtualPasteParts()
			next, cmd, err := m.executeCommand(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.closeCommandPalette()
		if shouldExecuteFromPalette(selected) || selected.Name == "/continue" {
			if selected.Name == "/quit" {
				return m, tea.Quit
			}
			if m.busy {
				m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
				return m, nil
			}
			m.input.Reset()
			m.clearPasteTransaction()
			m.clearVirtualPasteParts()
			next, cmd, err := m.executeCommand(selected.Name)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.setInputValue(selected.Usage)
		m.statusNote = selected.Description
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

func (m *model) applyMentionSelection(selected mention.Candidate) {
	m.recordRecentMention(selected.Path)

	if assetID, note, isImage := m.ingestMentionImageCandidate(selected.Path); isImage {
		if strings.TrimSpace(string(assetID)) != "" {
			m.bindMentionImageAsset(selected.Path, assetID)
			nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
			m.setInputValue(nextValue)
			if strings.TrimSpace(note) != "" {
				m.statusNote = note
			} else {
				m.statusNote = "Attached image: @" + filepath.ToSlash(strings.TrimSpace(selected.Path))
			}
			m.closeMentionPalette()
			m.syncInputOverlays()
			return
		}
		if strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}

	nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
	m.setInputValue(nextValue)
	m.statusNote = "Inserted mention: " + selected.Path
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) ingestMentionImageCandidate(path string) (assetID llm.AssetID, note string, isImage bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.workspace, resolved)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	if _, ok := mediaTypeFromPath(resolved); !ok {
		return "", "", false
	}

	placeholder, note, ok := m.ingestImageFromPath(resolved)
	if !ok {
		return "", note, true
	}
	imageID, ok := imageIDFromPlaceholder(placeholder)
	if !ok {
		return "", "image ingest failed: invalid placeholder id", true
	}
	assetID, _, ok = m.findAssetByImageID(imageID)
	if !ok {
		return "", "image ingest failed: asset metadata missing", true
	}
	return assetID, note, true
}

func (m *model) bindMentionImageAsset(path string, assetID llm.AssetID) {
	if m == nil {
		return
	}
	key := normalizeImageMentionPath(path)
	if key == "" || strings.TrimSpace(string(assetID)) == "" {
		return
	}
	if m.inputImageMentions == nil {
		m.inputImageMentions = make(map[string]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	if prev, ok := m.inputImageMentions[key]; ok && prev != assetID {
		m.orphanedImages[prev] = time.Now().UTC()
	}
	m.inputImageMentions[key] = assetID
	delete(m.orphanedImages, assetID)
}

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.skillsOpen = false
	m.commandCursor = 0
	m.setInputValue("/")
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) openPromptSearch(mode promptSearchMode) tea.Cmd {
	m.promptSearchMode = mode
	m.promptSearchBaseInput = m.input.Value()
	m.promptSearchQuery = ""
	m.promptSearchCursor = 0
	m.promptSearchOpen = true
	m.commandOpen = false
	m.closeMentionPalette()

	loadCmd := tea.Cmd(nil)
	if !m.promptHistoryLoaded && !m.promptHistoryLoading {
		m.promptHistoryLoading = true
		m.promptHistoryLoadErr = ""
		loadCmd = m.loadPromptHistoryCmd()
	}
	m.refreshPromptSearchMatches()

	switch {
	case m.promptHistoryLoading:
		if mode == promptSearchModePanel {
			m.statusNote = "History panel loading..."
		} else {
			m.statusNote = "Prompt history loading..."
		}
	case strings.TrimSpace(m.promptHistoryLoadErr) != "":
		m.statusNote = "Prompt history unavailable: " + compact(m.promptHistoryLoadErr, 72)
	case len(m.promptSearchMatches) == 0:
		if mode == promptSearchModePanel {
			m.statusNote = "History panel opened. No matching prompts."
		} else {
			m.statusNote = "No matching prompts."
		}
	default:
		if mode == promptSearchModePanel {
			m.statusNote = fmt.Sprintf("History panel ready (%d matches).", len(m.promptSearchMatches))
		} else {
			m.statusNote = fmt.Sprintf("Prompt history ready (%d matches).", len(m.promptSearchMatches))
		}
	}
	return loadCmd
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

func (m model) loadPromptHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := history.LoadRecentPrompts(promptSearchLoadLimit)
		return promptHistoryLoadedMsg{Entries: entries, Err: err}
	}
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

func (m *model) toggleMode() {
	if m.mode == modeBuild {
		m.mode = modePlan
		if m.plan.Phase == planpkg.PhaseNone {
			m.plan.Phase = planpkg.PhaseExplore
		}
		m.statusNote = "Switched to Plan mode. Explore, clarify, and converge the plan before executing."
	} else {
		m.mode = modeBuild
		m.statusNote = "Switched to Build mode. Use the current plan baseline to drive execution."
	}
	if m.sess != nil {
		m.sess.Mode = planpkg.NormalizeMode(string(m.mode))
		m.sess.Plan = copyPlanState(m.plan)
		if m.store != nil {
			_ = m.store.Save(m.sess)
		}
	}
	m.syncPlanActionPicker()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

func (m *model) closeCommandPalette() {
	m.commandOpen = false
	m.commandCursor = 0
	m.closeMentionPalette()
	m.input.Reset()
	m.clearPasteTransaction()
	m.clearVirtualPasteParts()
}

func (m model) selectedCommandItem() (commandItem, bool) {
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandItem{}, false
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	return items[index], true
}
