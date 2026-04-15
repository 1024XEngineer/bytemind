package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleSlashCommand(input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/help":
		m.screen = screenChat
		m.appendChat(chatEntry{
			Kind:   "user",
			Title:  "You",
			Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
			Body:   input,
			Status: "final",
		})
		m.appendChat(chatEntry{Kind: "assistant", Title: assistantLabel, Body: m.helpText(), Status: "final"})
		m.statusNote = "Help opened in the conversation view."
		return nil
	case "/session":
		m.sessionsOpen = true
		m.statusNote = "Opened recent sessions."
		return nil
	case "/skills-select":
		return m.openSkillsPicker()
	case "/skills":
		return m.runSkillsListCommand(input)
	case "/skill":
		return m.runSkillCommand(input, fields)
	case "/new":
		return m.newSession()
	case "/compact":
		return m.runCompactCommand(input)
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
}

func (m model) executeCommand(input string) (tea.Model, tea.Cmd, error) {
	if err := m.handleSlashCommand(input); err != nil {
		return m, nil, err
	}
	m.refreshViewport()
	return m, m.loadSessionsCmd(), nil
}
