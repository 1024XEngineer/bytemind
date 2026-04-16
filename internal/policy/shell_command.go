package policy

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

type ShellRisk string

const (
	ShellRiskSafe     ShellRisk = "safe"
	ShellRiskApproval ShellRisk = "approval"
	ShellRiskBlocked  ShellRisk = "blocked"
)

type ShellAssessment struct {
	Risk   ShellRisk
	Reason string
}

func AssessShellCommand(command string) ShellAssessment {
	segments := splitCommandSegments(command)
	result := ShellAssessment{Risk: ShellRiskSafe}
	for _, segment := range segments {
		assessment := assessCommandSegment(segment)
		if shellRiskOrder(assessment.Risk) > shellRiskOrder(result.Risk) {
			result = assessment
		}
		if result.Risk == ShellRiskBlocked {
			return result
		}
	}
	return result
}

func IsPlanSafeShellCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if hasWriteRedirection(command) {
		return false
	}
	segments := splitCommandSegments(command)
	if len(segments) != 1 {
		return false
	}
	fields := splitCommandFields(segments[0])
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(fields[0]))
	if first == "" {
		return false
	}
	if looksLikeScript(first) {
		return false
	}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		switch trimmed {
		case "|", ">", ">>", "<", ";", "&&", "||":
			return false
		}
	}

	switch first {
	case "ls", "dir", "pwd", "cat", "type", "rg", "grep", "find", "tree":
		return true
	case "git":
		if len(fields) < 2 {
			return false
		}
		sub := strings.ToLower(fields[1])
		return sub == "status" || sub == "diff" || sub == "log"
	case "go":
		if len(fields) < 2 {
			return false
		}
		sub := strings.ToLower(fields[1])
		return sub == "env" || sub == "list"
	case "bash", "sh", "pwsh", "powershell", "python", "python3", "node":
		return false
	default:
		return false
	}
}

func shellRiskOrder(risk ShellRisk) int {
	switch risk {
	case ShellRiskBlocked:
		return 3
	case ShellRiskApproval:
		return 2
	default:
		return 1
	}
}

func assessCommandSegment(segment string) ShellAssessment {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ShellAssessment{Risk: ShellRiskSafe}
	}
	if hasWriteRedirection(segment) {
		return ShellAssessment{Risk: ShellRiskApproval, Reason: "uses shell redirection"}
	}

	fields := splitCommandFields(segment)
	if len(fields) == 0 {
		return ShellAssessment{Risk: ShellRiskSafe}
	}

	command := strings.ToLower(fields[0])
	if isBlockedCommand(command, fields) {
		return ShellAssessment{Risk: ShellRiskBlocked, Reason: fmt.Sprintf("blocked dangerous shell command: %s", strings.TrimSpace(segment))}
	}
	if isReadOnlyCommand(command, fields) {
		return ShellAssessment{Risk: ShellRiskSafe}
	}
	if isApprovalCommand(command, fields) {
		return ShellAssessment{Risk: ShellRiskApproval, Reason: fmt.Sprintf("may modify files or environment: %s", fields[0])}
	}
	return ShellAssessment{Risk: ShellRiskApproval, Reason: fmt.Sprintf("requires approval for non-read-only command: %s", fields[0])}
}

func splitCommandSegments(command string) []string {
	normalized := strings.ReplaceAll(command, "\r\n", "\n")
	segments := make([]string, 0, 4)
	var builder strings.Builder
	inSingle := false
	inDouble := false

	flush := func() {
		segment := strings.TrimSpace(builder.String())
		if segment != "" {
			segments = append(segments, segment)
		}
		builder.Reset()
	}

	for i := 0; i < len(normalized); i++ {
		ch := normalized[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			builder.WriteByte(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			builder.WriteByte(ch)
		case '\n', ';':
			if inSingle || inDouble {
				builder.WriteByte(ch)
				continue
			}
			flush()
		case '|', '&':
			if inSingle || inDouble {
				builder.WriteByte(ch)
				continue
			}
			flush()
			if i+1 < len(normalized) && normalized[i+1] == ch {
				i++
			}
		default:
			builder.WriteByte(ch)
		}
	}
	flush()
	return segments
}

func splitCommandFields(segment string) []string {
	fields := make([]string, 0, 8)
	var builder strings.Builder
	inSingle := false
	inDouble := false

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		fields = append(fields, builder.String())
		builder.Reset()
	}

	for i := 0; i < len(segment); i++ {
		ch := segment[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				continue
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
				continue
			}
		}
		if !inSingle && !inDouble && unicode.IsSpace(rune(ch)) {
			flush()
			continue
		}
		builder.WriteByte(ch)
	}
	flush()
	return fields
}

func hasWriteRedirection(segment string) bool {
	inSingle := false
	inDouble := false
	for i := 0; i < len(segment); i++ {
		ch := segment[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '>':
			if !inSingle && !inDouble {
				return true
			}
		}
	}
	return false
}

func isBlockedCommand(command string, fields []string) bool {
	switch command {
	case "rm", "rmdir", "del", "erase", "remove-item", "ri", "rd", "format", "diskpart", "mkfs", "dd", "shutdown", "reboot", "halt", "poweroff":
		return true
	case "git":
		return isBlockedGit(fields)
	default:
		return false
	}
}

func isBlockedGit(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	sub := strings.ToLower(fields[1])
	switch sub {
	case "reset":
		return hasAnyArg(fields[2:], "--hard")
	case "clean":
		for _, arg := range fields[2:] {
			if strings.HasPrefix(arg, "-f") || strings.Contains(arg, "f") && strings.HasPrefix(arg, "-") {
				return true
			}
		}
	case "checkout":
		return hasAnyArg(fields[2:], "--")
	case "restore":
		return len(fields) > 2
	}
	return false
}

func isReadOnlyCommand(command string, fields []string) bool {
	switch command {
	case "cat", "type", "ls", "dir", "pwd", "echo", "rg", "grep", "find", "where", "which", "env", "printenv", "uname", "whoami", "head", "tail", "sort", "uniq", "wc", "tree", "get-childitem", "get-content", "select-string", "get-location", "resolve-path":
		return true
	case "git":
		return isReadOnlyGit(fields)
	case "go":
		return len(fields) > 1 && isOneOf(strings.ToLower(fields[1]), "env", "list", "version")
	case "npm", "pnpm", "yarn":
		return len(fields) > 1 && isOneOf(strings.ToLower(fields[1]), "list", "info", "view", "why")
	default:
		return false
	}
}

func isApprovalCommand(command string, fields []string) bool {
	switch command {
	case "cp", "copy", "copy-item", "mv", "move", "move-item", "rename", "rename-item", "new-item", "mkdir", "md", "touch", "tee", "set-content", "add-content", "out-file":
		return true
	case "git":
		return len(fields) > 1
	case "go":
		return len(fields) > 1 && isOneOf(strings.ToLower(fields[1]), "test", "build", "run", "mod", "get")
	case "npm", "pnpm", "yarn":
		return len(fields) > 1 && isOneOf(strings.ToLower(fields[1]), "install", "add", "remove", "update", "run")
	case "pip", "pip3", "uv", "cargo", "make", "cmake", "python", "python3", "node", "pwsh", "powershell", "sh", "bash":
		return true
	default:
		return false
	}
}

func isReadOnlyGit(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	sub := strings.ToLower(fields[1])
	return isOneOf(sub, "status", "diff", "log", "show", "rev-parse", "ls-files", "grep", "branch") && len(fields) == 2
}

func hasAnyArg(args []string, targets ...string) bool {
	for _, arg := range args {
		for _, target := range targets {
			if strings.EqualFold(arg, target) {
				return true
			}
		}
	}
	return false
}

func isOneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func looksLikeScript(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if strings.HasPrefix(command, "./") || strings.HasPrefix(command, ".\\") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(command))
	switch ext {
	case ".sh", ".ps1", ".bat", ".cmd":
		return true
	default:
		return false
	}
}
