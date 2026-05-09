package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleSlashCommand(input string) error {
	raw := strings.TrimSpace(input)

	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/add":
		return m.runAddCommand(input, fields)
	case "/delete":
		return m.runDeleteCommand(input, fields)
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
		return m.openSessionsModal()
	case "/skills-select":
		return m.openSkillsPicker()
	case "/skills":
		return m.runSkillsListCommand(input)
	case "/skill":
		return m.runSkillCommand(input, fields)
	case "/agents":
		return m.runAgentsCommand(input)
	case "/mcp":
		return m.runMCPCommandDispatch(input, fields)
	case "/models":
		return m.runModelsCommand(input, fields)
	case "/model":
		return m.runModelCommand(input, fields)
	case "/new":
		return m.newSession()
	case "/compact":
		return m.runCompactCommand(input)
	case "/commit":
		return m.runCommitCommand(input)
	case "/undo-commit":
		return m.runUndoCommitCommand(input)
	case "/rollback":
		return m.runRollbackCommand(input)
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
}

func (m model) executeCommand(input string) (tea.Model, tea.Cmd, error) {
	previousScreen := m.screen
	if err := m.handleSlashCommand(input); err != nil {
		return m, nil, err
	}
	commandRunCmd := m.pendingCommandCmd
	m.pendingCommandCmd = nil
	m.refreshViewport()
	cmds := []tea.Cmd{m.loadSessionsCmd(), m.startLandingGlowOnTransition(previousScreen)}
	if commandRunCmd != nil {
		cmds = append(cmds, commandRunCmd)
	}
	return m, tea.Batch(cmds...), nil
}
