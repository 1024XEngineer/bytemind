package tui

import (
	"fmt"
	"strings"
)

func (m *model) runSkillsListCommand(input string) error {
	_ = m.refreshSkillCatalog()
	skillsList := m.skillCatalog.Items
	diagnostics := m.skillCatalog.Diagnostics
	active := m.activeSkillSnapshot()

	lines := make([]string, 0, len(skillsList)+8)
	if active != nil {
		lines = append(lines, fmt.Sprintf("Active skill: %s (%s)", active.Name, active.Scope))
	} else {
		lines = append(lines, "Active skill: none")
	}
	lines = append(lines, "")
	if len(skillsList) == 0 {
		lines = append(lines, "No skills discovered.")
	} else {
		lines = append(lines, "Available skills:")
		for _, skill := range skillsList {
			lines = append(lines, fmt.Sprintf("- %s (%s): %s", skill.Name, skill.Scope, skill.Description))
		}
	}
	if len(diagnostics) > 0 {
		lines = append(lines, "", "Diagnostics:")
		for _, diag := range diagnostics {
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s): %s", diag.Level, diag.Skill, diag.Path, diag.Message))
		}
	}

	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = fmt.Sprintf("Discovered %d skill(s).", len(skillsList))
	return nil
}

func (m *model) openSkillsPicker() error {
	if err := m.refreshSkillCatalog(); err != nil {
		return err
	}
	if len(m.skillCatalog.Items) == 0 {
		return fmt.Errorf("no skills available")
	}
	m.skillsOpen = true
	m.commandOpen = false
	m.sessionsOpen = false
	m.commandCursor = 0
	return nil
}

func (m *model) runSkillCommand(input string, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("usage: /skill <clear|delete> ...")
	}
	switch strings.ToLower(strings.TrimSpace(fields[1])) {
	case "clear":
		return m.runSkillStateClearCommand(input, fields)
	case "delete":
		return m.runSkillDeleteCommand(input, fields)
	default:
		return fmt.Errorf("usage: /skill <clear|delete> ...")
	}
}

func (m *model) runSkillStateClearCommand(input string, fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("usage: /skill clear")
	}

	result := m.skillsManager().Clear()
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	_ = m.refreshSkillCatalog()

	message := result.Data
	m.appendCommandExchange(input, message)
	m.statusNote = "Skill state cleared"
	return nil
}

func (m *model) runSkillDeleteCommand(input string, fields []string) error {
	if len(fields) < 3 {
		return fmt.Errorf("usage: /skill delete <name>")
	}
	name := strings.TrimSpace(strings.TrimPrefix(fields[2], "/"))
	if name == "" {
		return fmt.Errorf("usage: /skill delete <name>")
	}

	result := m.skillsManager().Delete(name)
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}
	_ = m.refreshSkillCatalog()

	lines := []string{
		fmt.Sprintf("Deleted project skill `%s`.", result.Data.Name),
		fmt.Sprintf("Dir: %s", result.Data.Dir),
	}

	if result.Data.ClearedActive {
		lines = append(lines, "Cleared active skill in this session as well.")
	}
	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = "Skill deleted"
	return nil
}
