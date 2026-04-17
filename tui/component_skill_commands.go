package tui

import (
	"fmt"
	"strings"
)

func (m *model) runSkillsListCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skillsList, diagnostics := m.runner.ListSkills()
	active, hasActive := m.runner.GetActiveSkill(m.sess)

	lines := make([]string, 0, len(skillsList)+8)
	if hasActive {
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
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skillsList, _ := m.runner.ListSkills()
	if len(skillsList) == 0 {
		m.statusNote = "No loaded skills available."
		return nil
	}
	m.skillsOpen = true
	m.commandOpen = false
	m.sessionsOpen = false
	m.commandCursor = 0
	m.statusNote = "Opened loaded skills picker."
	return nil
}

func (m *model) runSkillCommand(input string, fields []string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
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

	activeName := ""
	if m.sess != nil && m.sess.ActiveSkill != nil {
		activeName = strings.TrimSpace(m.sess.ActiveSkill.Name)
	}
	if err := m.runner.ClearActiveSkill(m.sess); err != nil {
		return err
	}

	message := "No active skill in this session; state remains empty."
	if activeName != "" {
		message = fmt.Sprintf("Cleared active skill `%s` from this session.", activeName)
	}
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

	result, err := m.runner.ClearSkill(name)
	if err != nil {
		return err
	}

	lines := []string{
		fmt.Sprintf("Deleted project skill `%s`.", result.Name),
		fmt.Sprintf("Dir: %s", result.Dir),
	}

	if m.sess != nil && m.sess.ActiveSkill != nil && strings.EqualFold(strings.TrimSpace(m.sess.ActiveSkill.Name), strings.TrimSpace(result.Name)) {
		if clearErr := m.runner.ClearActiveSkill(m.sess); clearErr == nil {
			lines = append(lines, "Cleared active skill in this session as well.")
		}
	}
	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = "Skill deleted"
	return nil
}
