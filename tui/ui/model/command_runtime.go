package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (m *model) runCompactCommand(input string) error {
	result, err := m.runtimeAPI().CompactSession(m.sess)
	if err != nil {
		return err
	}
	if !result.Changed {
		m.appendCommandExchange(input, "No compaction needed yet. Start a longer conversation first.")
		m.statusNote = "No compaction needed."
		return nil
	}
	preview := compact(result.Summary, 360)
	response := "Conversation compacted for long-context continuation."
	if strings.TrimSpace(preview) != "" {
		response += "\nSummary preview: " + preview
	}
	m.chatItems, m.toolRuns = rebuildSessionTimeline(m.sess)
	m.appendCommandExchange(input, response)
	m.statusNote = "Conversation compacted."
	return nil
}

func (m *model) runDirectSkillCommand(input string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	name := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(name, "/") || !m.isKnownSkillCommand(name) {
		return fmt.Errorf("unknown command: %s", fields[0])
	}
	args, err := parseSkillArgs(fields[1:])
	if err != nil {
		return err
	}
	return m.activateSkillCommand(input, name, args)
}

func parseSkillArgs(parts []string) (map[string]string, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	args := make(map[string]string, len(parts))
	for _, part := range parts {
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		key := strings.TrimSpace(pieces[0])
		value := strings.TrimSpace(pieces[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		args[key] = value
	}
	if len(args) == 0 {
		return nil, nil
	}
	return args, nil
}

func (m *model) appendCommandExchange(command, response string) {
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   command,
		Status: "final",
	})
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   response,
		Status: "final",
	})
}

func (m *model) activateSkillCommand(input, name string, args map[string]string) error {
	skill := m.skillsManager().Activate(name, args)
	if !skill.Success {
		return fmt.Errorf("%s", skill.Error)
	}
	_ = m.refreshSkillCatalog()
	response := fmt.Sprintf("Activated skill `%s` (%s).\nTool policy: %s\nEntry: %s", skill.Data.Name, skill.Data.Scope, skill.Data.ToolPolicy, skill.Data.EntrySlash)
	if len(args) > 0 {
		argParts := make([]string, 0, len(args))
		keys := make([]string, 0, len(args))
		for key := range args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			argParts = append(argParts, fmt.Sprintf("%s=%s", key, args[key]))
		}
		response += "\nArgs: " + strings.Join(argParts, ", ")
	}
	m.appendCommandExchange(input, response)
	m.statusNote = "Skill activated."
	return nil
}

func (m *model) activateSelectedSkill() error {
	items := m.skillPickerItems()
	if len(items) == 0 {
		return nil
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	selected := items[index]
	return m.activateSkillCommand(selected.Usage, selected.Usage, nil)
}
