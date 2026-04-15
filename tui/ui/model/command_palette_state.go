package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

func (m *model) skillPickerItems() []commandItem {
	if m != nil && len(m.skillCatalog.Items) == 0 && m.runner != nil {
		_ = m.refreshSkillCatalog()
	}
	if len(m.skillCatalog.Items) == 0 {
		return nil
	}

	items := make([]commandItem, 0, len(m.skillCatalog.Items))
	for _, skill := range m.skillCatalog.Items {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
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

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.skillsOpen = false
	m.commandCursor = 0
	m.setInputValue("/")
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) closeCommandPalette() {
	m.commandOpen = false
	m.commandCursor = 0
	m.closeMentionPalette()
	m.input.Reset()
}

func (m model) selectedCommandItem() (commandItem, bool) {
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandItem{}, false
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	return items[index], true
}

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}
