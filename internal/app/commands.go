package app

import "strings"

type SlashCommand struct {
	Name        string
	Usage       string
	Description string
}

var defaultSlashCommands = []SlashCommand{
	{Name: "/help", Usage: "/help", Description: "Show available commands"},
	{Name: "/session", Usage: "/session", Description: "Show the current session"},
	{Name: "/sessions", Usage: "/sessions [limit]", Description: "List recent sessions"},
	{Name: "/resume", Usage: "/resume <id>", Description: "Resume a recent session by id or prefix (CLI only)"},
	{Name: "/new", Usage: "/new", Description: "Start a new session in the current workspace"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the CLI"},
}

func DefaultSlashCommands() []SlashCommand {
	out := make([]SlashCommand, len(defaultSlashCommands))
	copy(out, defaultSlashCommands)
	return out
}

func MatchSlashCommands(prefix string, commands []SlashCommand) []string {
	matches := make([]string, 0, len(commands))
	for _, cmd := range commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd.Name)
		}
	}
	return matches
}

func ContainsCommand(commands []string, target string) bool {
	for _, cmd := range commands {
		if cmd == target {
			return true
		}
	}
	return false
}

func CompleteSlashCommand(input string, commands []SlashCommand) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return input, nil
	}

	matches := MatchSlashCommands(fields[0], commands)
	if len(matches) == 0 {
		return input, nil
	}
	if len(matches) > 1 && !ContainsCommand(matches, fields[0]) {
		return input, matches
	}
	if ContainsCommand(matches, fields[0]) {
		return input, nil
	}

	completed := matches[0]
	if len(fields) == 1 {
		return completed, nil
	}
	return completed + " " + strings.Join(fields[1:], " "), nil
}

func CommandNames(commands []SlashCommand) []string {
	items := make([]string, 0, len(commands))
	for _, cmd := range commands {
		items = append(items, cmd.Name)
	}
	return items
}
