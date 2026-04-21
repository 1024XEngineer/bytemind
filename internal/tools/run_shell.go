package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	policypkg "bytemind/internal/policy"
)

type RunShellTool struct{}

type shellRisk = policypkg.ShellRisk

const (
	shellRiskSafe     shellRisk = policypkg.ShellRiskSafe
	shellRiskApproval shellRisk = policypkg.ShellRiskApproval
	shellRiskBlocked  shellRisk = policypkg.ShellRiskBlocked
)

type shellAssessment = policypkg.ShellAssessment

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
	if execCtx != nil && execCtx.SkipShellApproval {
		return nil
	}
	mode := planpkg.ModeBuild
	approvalPolicy := ""
	if execCtx != nil {
		mode = planpkg.NormalizeMode(string(execCtx.Mode))
		approvalPolicy = strings.TrimSpace(execCtx.ApprovalPolicy)
	}
	if mode == planpkg.ModePlan {
		if !isPlanSafeCommand(command) {
			return errors.New("shell command is unavailable in plan mode unless it matches the strict read-only allowlist")
		}
		return nil
	}

	assessment := assessShellCommand(command)
	if assessment.Risk == shellRiskBlocked {
		return errors.New(assessment.Reason)
	}

	switch approvalPolicy {
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

func isPlanSafeCommand(command string) bool {
	return policypkg.IsPlanSafeShellCommand(command)
}

func promptForApproval(command, reason string, execCtx *ExecutionContext) error {
	if execCtx != nil && execCtx.isAwayMode() {
		return awayModeApprovalDeniedError("shell command", command, execCtx)
	}
	if execCtx == nil {
		return approvalChannelUnavailableError("shell command", command)
	}
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
		return approvalChannelUnavailableError("shell command", command)
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
	return policypkg.AssessShellCommand(command)
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		executable := resolveWindowsShellExecutable(exec.LookPath, os.Stat, os.Getenv)
		return exec.CommandContext(ctx, executable, "-NoProfile", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}

func resolveWindowsShellExecutable(
	lookPath func(file string) (string, error),
	statFn func(name string) (os.FileInfo, error),
	getenv func(key string) string,
) string {
	for _, candidate := range windowsShellCandidates(getenv) {
		if isWindowsAbsolutePath(candidate) {
			info, err := statFn(candidate)
			if err == nil && info != nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		resolved, err := lookPath(candidate)
		if err == nil && strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	return "powershell"
}

func windowsShellCandidates(getenv func(key string) string) []string {
	candidates := []string{
		"powershell.exe",
		"powershell",
		"pwsh.exe",
		"pwsh",
	}

	appendWindowsRoot := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		candidates = append(candidates,
			filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
			filepath.Join(root, "Sysnative", "WindowsPowerShell", "v1.0", "powershell.exe"),
		)
	}

	appendWindowsRoot(getenv("SystemRoot"))
	appendWindowsRoot(getenv("WINDIR"))

	if programFiles := strings.TrimSpace(getenv("ProgramFiles")); programFiles != "" {
		candidates = append(candidates, filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"))
	}

	seen := make(map[string]struct{}, len(candidates))
	uniq := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, candidate)
	}
	return uniq
}

func isWindowsAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
		return true
	}
	return false
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
