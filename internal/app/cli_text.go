package app

type HelpLine struct {
	Usage       string
	Description string
}

func DefaultUsageLines() []string {
	return []string{
		"bytemind chat [-config path] [-model name] [-session id] [-stream true|false] [-sandbox-enabled true|false] [-system-sandbox-mode off|best_effort|required] [-approval-mode interactive|away] [-away-policy auto_deny_continue|fail_fast] [-workspace path] [-max-iterations n]",
		"bytemind tui [-config path] [-model name] [-session id] [-stream true|false] [-sandbox-enabled true|false] [-system-sandbox-mode off|best_effort|required] [-approval-mode interactive|away] [-away-policy auto_deny_continue|fail_fast] [-workspace path] [-max-iterations n]",
		`bytemind run -prompt "task" [-config path] [-model name] [-session id] [-stream true|false] [-sandbox-enabled true|false] [-system-sandbox-mode off|best_effort|required] [-approval-mode interactive|away] [-away-policy auto_deny_continue|fail_fast] [-max-iterations n]`,
		"bytemind install [-to dir] [-name binary-name]",
		"bytemind mcp <list|add|remove|enable|disable|test|reload> [options]",
		"tip: install without Go (macOS/Linux): curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash",
		"tip: install without Go (Windows): iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex",
	}
}

func DefaultHelpLines() []HelpLine {
	commands := DefaultSlashCommands()
	lines := make([]HelpLine, 0, len(commands))
	for _, cmd := range commands {
		lines = append(lines, HelpLine{
			Usage:       cmd.Usage,
			Description: cmd.Description,
		})
	}
	return lines
}
