package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"bytemind/internal/session"
)

func (m *model) runCompactCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	if m.sess == nil {
		return fmt.Errorf("session is unavailable")
	}
	type sessionCompactor interface {
		CompactSession(ctx context.Context, sess *session.Session) (string, bool, error)
	}
	compactor, ok := any(m.runner).(sessionCompactor)
	if !ok {
		return fmt.Errorf("compact is unavailable in this build")
	}
	summary, changed, err := compactor.CompactSession(context.Background(), m.sess)
	if err != nil {
		return err
	}
	if !changed {
		m.appendCommandExchange(input, "No compaction needed yet. Start a longer conversation first.")
		m.statusNote = "No compaction needed."
		return nil
	}
	preview := compact(summary, 360)
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
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skill, err := m.runner.ActivateSkill(m.sess, name, args)
	if err != nil {
		return err
	}
	response := fmt.Sprintf("Activated skill `%s` (%s).\nTool policy: %s\nEntry: %s", skill.Name, skill.Scope, skill.ToolPolicy.Policy, skill.Entry.Slash)
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
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	items := m.skillPickerItems()
	if len(items) == 0 {
		return nil
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	selected := items[index]
	return m.activateSkillCommand(selected.Usage, selected.Usage, nil)
}
