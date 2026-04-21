package tui

import "strings"

func (m model) helpText() string {
	return strings.Join([]string{
		"# Bytemind Help",
		"",
		"## Entry Points",
		"- Run `go run ./cmd/bytemind chat` from the repository root to open the TUI.",
		"- `chat` opens the landing screen first, then enters conversation view after you submit a prompt.",
		"- Run `go run ./cmd/bytemind run -prompt \"...\"` for one-shot execution.",
		"",
		"## Slash Commands",
		"- `/help`: show this help inside the conversation.",
		"- `/session`: open recent sessions.",
		"- `/skills`: list discovered skills and diagnostics.",
		"- `/<skill-name> [k=v...]`: activate a skill for this session.",
		"- `/skill clear`: clear the active skill in this session.",
		"- `/skill delete <name>`: delete the specified project skill.",
		"- `/new`: start a fresh session.",
		"- `/compact`: summarize long history into a compact continuation context.",
		"- `/btw <message>`: interject while a run is in progress.",
		"- `/quit`: exit the TUI.",
		"- TUI does not expose `/resume`; use `/session` then `Enter` on the selected row.",
		"- CLI keeps `/resume <id>` for command-line or scripted recovery.",
		"",
		"## UI Notes",
		"- `Tab` toggles between Build and Plan modes.",
		"- Plan mode keeps the plan panel visible and focused on structured steps.",
		"- `Ctrl+G` opens or closes the help panel.",
		"- `Ctrl+A` toggles away mode (`Away:ON/OFF`) for approval handling.",
		"- Drag across the conversation with the left mouse button, then press `Ctrl+C` to copy.",
		"- If provider setup is required, paste an API key in the input and press Enter.",
		"- Long pasted code/text is compressed to `[Paste #N ~X lines]`.",
		"- Use `[Paste]`, `[Paste #N]`, `[Paste line3]`, or `[Paste #N line3~line7]` to expand references.",
		"- After restoring a session with a saved plan, type `continue execution` to resume it.",
		"- Approval requests appear above the input area when a shell command needs confirmation.",
		"- Footer shortcuts: `tab` agents, `/` commands, drag select, `Ctrl+A` away, `Ctrl+C` copy/quit, `Ctrl+L` sessions.",
	}, "\n")
}

func visibleCommandItems(group string) []commandItem {
	items := make([]commandItem, 0, len(commandItems))
	for _, item := range commandItems {
		if group == "" {
			if item.Kind == "group" || item.Group == "" {
				items = append(items, item)
			}
			continue
		}
		if item.Kind == "command" && item.Group == group {
			items = append(items, item)
		}
	}
	return items
}

func normalizeSkillCommand(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimLeft(name, "/")
	return strings.TrimSpace(name)
}

func commandFilterQuery(value, group string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	value = strings.TrimPrefix(value, "/")
	if group != "" {
		if strings.HasPrefix(value, group) {
			value = strings.TrimSpace(strings.TrimPrefix(value, group))
		}
	}
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "/")))
}

func matchesCommandItem(item commandItem, query string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	name := strings.ToLower(strings.TrimPrefix(item.Name, "/"))
	usage := strings.ToLower(strings.TrimPrefix(item.Usage, "/"))
	return strings.HasPrefix(name, query) ||
		strings.HasPrefix(usage, query)
}

func matchAllTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func parsePromptSearchQuery(raw string) (tokens []string, workspaceFilter, sessionFilter string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	tokens = make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		switch {
		case strings.HasPrefix(field, "ws:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "ws:"))
		case strings.HasPrefix(field, "workspace:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "workspace:"))
		case strings.HasPrefix(field, "sid:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "sid:"))
		case strings.HasPrefix(field, "session:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "session:"))
		default:
			tokens = append(tokens, field)
		}
	}
	return tokens, workspaceFilter, sessionFilter
}

func (m model) isKnownSkillCommand(command string) bool {
	if m.runner == nil {
		return false
	}
	normalized := normalizeSkillCommand(command)
	if normalized == "" {
		return false
	}
	skillsList, _ := m.runner.ListSkills()
	for _, skill := range skillsList {
		if normalizeSkillCommand(skill.Name) == normalized {
			return true
		}
		if normalizeSkillCommand(skill.Entry.Slash) == normalized {
			return true
		}
		for _, alias := range skill.Aliases {
			if normalizeSkillCommand(alias) == normalized {
				return true
			}
		}
	}
	return false
}
