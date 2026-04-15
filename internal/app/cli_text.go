package app

type HelpLine struct {
	Usage       string
	Description string
}

func DefaultUsageLines() []string {
	return []string{
		"bytemind chat [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]",
		"bytemind tui [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]",
		`bytemind run -prompt "task" [-config path] [-model name] [-session id] [-stream true|false] [-max-iterations n]`,
		"bytemind install [-to dir] [-name binary-name]",
		"tip: first time use `go run ./cmd/bytemind install`, then add install dir to PATH",
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
