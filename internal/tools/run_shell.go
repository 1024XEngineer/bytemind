package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"bytemind/internal/llm"
)

type RunShellTool struct{}

type shellRisk int

const (
	shellRiskSafe shellRisk = iota
	shellRiskApproval
	shellRiskBlocked
)

type shellAssessment struct {
	Risk   shellRisk
	Reason string
}

func (RunShellTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "run_shell",
			Description: "Run a shell command in the workspace",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Command to execute in the workspace shell.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds, defaults to 30 and caps at 300.",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

func (RunShellTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", errors.New("command is required")
	}
	if err := requireApproval(args.Command, execCtx); err != nil {
		return "", err
	}

	timeout := 30 * time.Second
	if args.TimeoutSeconds > 0 {
		if args.TimeoutSeconds > 300 {
			args.TimeoutSeconds = 300
		}
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := shellCommand(runCtx, args.Command)
	cmd.Dir = execCtx.Workspace

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return "", errors.New("command timed out")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", err
		}
	}

	return toJSON(map[string]any{
		"ok":        exitCode == 0,
		"exit_code": exitCode,
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
	})
}

func requireApproval(command string, execCtx *ExecutionContext) error {
	assessment := assessShellCommand(command)
	if assessment.Risk == shellRiskBlocked {
		return errors.New(assessment.Reason)
	}

	switch execCtx.ApprovalPolicy {
	case "never":
		return nil
	case "always":
		return promptForApproval(command, assessment.Reason, execCtx)
	default:
		if assessment.Risk == shellRiskApproval {
			return promptForApproval(command, assessment.Reason, execCtx)
		}
		return nil
	}
}

func promptForApproval(command, reason string, execCtx *ExecutionContext) error {
	if execCtx.Approval != nil {
		approved, err := execCtx.Approval(ApprovalRequest{
			Command: command,
			Reason:  reason,
		})
		if err != nil {
			return err
		}
		if !approved {
			return errors.New("shell command was not run because approval was denied")
		}
		return nil
	}
	if execCtx.Stdin == nil {
		return errors.New("shell command requires approval but no stdin is available")
	}
	if execCtx.Stdout != nil {
		if strings.TrimSpace(reason) != "" {
			fmt.Fprintf(execCtx.Stdout, "Approve shell command (%s) %q? [y/N]: ", reason, command)
		} else {
			fmt.Fprintf(execCtx.Stdout, "Approve shell command %q? [y/N]: ", command)
		}
	}
	reader := bufio.NewReader(execCtx.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return errors.New("shell command was not run because approval was denied")
	}
	return nil
}

func assessShellCommand(command string) shellAssessment {
	segments := splitCommandSegments(command)
	result := shellAssessment{Risk: shellRiskSafe}
	for _, segment := range segments {
		assessment := assessCommandSegment(segment)
		if assessment.Risk > result.Risk {
			result = assessment
		}
		if result.Risk == shellRiskBlocked {
			return result
		}
	}
	return result
}

func assessCommandSegment(segment string) shellAssessment {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return shellAssessment{Risk: shellRiskSafe}
	}
	if hasWriteRedirection(segment) {
		return shellAssessment{Risk: shellRiskApproval, Reason: "uses shell redirection"}
	}

	fields := splitCommandFields(segment)
	if len(fields) == 0 {
		return shellAssessment{Risk: shellRiskSafe}
	}

	command := strings.ToLower(fields[0])
	if isBlockedCommand(command, fields) {
		return shellAssessment{Risk: shellRiskBlocked, Reason: fmt.Sprintf("blocked dangerous shell command: %s", strings.TrimSpace(segment))}
	}
	if isReadOnlyCommand(command, fields) {
		return shellAssessment{Risk: shellRiskSafe}
	}
	if isApprovalCommand(command, fields) {
		return shellAssessment{Risk: shellRiskApproval, Reason: fmt.Sprintf("may modify files or environment: %s", fields[0])}
	}
	return shellAssessment{Risk: shellRiskApproval, Reason: fmt.Sprintf("requires approval for non-read-only command: %s", fields[0])}
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

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}
