package policy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"forgecli/internal/config"
)

type Policy struct {
	denylist          map[string]struct{}
	sensitivePatterns []string
}

func New(cfg config.Config) Policy {
	denylist := make(map[string]struct{}, len(cfg.DangerousCmdDenylist))
	for _, item := range cfg.DangerousCmdDenylist {
		denylist[strings.ToLower(item)] = struct{}{}
	}

	return Policy{
		denylist:          denylist,
		sensitivePatterns: cfg.SensitivePatterns,
	}
}

func (p Policy) EnsureEditablePath(path string) error {
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, pattern := range p.sensitivePatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return fmt.Errorf("refusing to edit sensitive file: %s", path)
		}
	}
	return nil
}

func (p Policy) ValidateCommand(commandLine string) error {
	commandLine = strings.TrimSpace(commandLine)
	if commandLine == "" {
		return nil
	}

	for _, token := range []string{"&&", "||", "|", ";", ">", "<", "$(", "`"} {
		if strings.Contains(commandLine, token) {
			return fmt.Errorf("command contains blocked shell token %q", token)
		}
	}

	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return errors.New("command is empty")
	}

	for _, field := range fields {
		fieldLower := strings.ToLower(field)
		if _, blocked := p.denylist[fieldLower]; blocked {
			return fmt.Errorf("dangerous command is blocked: %s", field)
		}
	}

	return nil
}
