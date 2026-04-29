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

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	policypkg "github.com/1024XEngineer/bytemind/internal/policy"
)

type RunShellTool struct{}

type shellRisk = policypkg.ShellRisk

const (
	shellRiskSafe     shellRisk = policypkg.ShellRiskSafe
	shellRiskApproval shellRisk = policypkg.ShellRiskApproval
	shellRiskBlocked  shellRisk = policypkg.ShellRiskBlocked
)

type shellAssessment = policypkg.ShellAssessment

const (
	systemSandboxModeOff        = "off"
	systemSandboxModeBestEffort = "best_effort"
	systemSandboxModeRequired   = "required"
)

var runShellLookPath = exec.LookPath

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

	cmd, sandboxBackend, sandboxMode, err := shellCommand(runCtx, args.Command, execCtx)
	if err != nil {
		if sandboxMode == systemSandboxModeRequired {
			return "", NewToolExecError(ToolErrorPermissionDenied, strings.TrimSpace(err.Error()), false, err)
		}
		return "", err
	}
	cmd.Dir = execCtx.Workspace

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = runCommandWithSystemSandbox(cmd, sandboxBackend.Name, sandboxMode)
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
		"ok":             exitCode == 0,
		"exit_code":      exitCode,
		"stdout":         stdout.String(),
		"stderr":         stderr.String(),
		"system_sandbox": buildSystemSandboxExecutionMetadata(sandboxMode, sandboxBackend),
	})
}

func buildSystemSandboxExecutionMetadata(mode string, backend systemSandboxRuntimeBackend) map[string]any {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = systemSandboxModeOff
	}
	backendName := strings.TrimSpace(backend.Name)
	active := backend.Enabled
	if !active {
		backendName = "none"
	}
	reason := strings.TrimSpace(backend.UnavailableReason)
	fallback := mode != systemSandboxModeOff && !active && reason != ""
	status := "inactive"
	if active {
		status = "active"
	} else if fallback {
		status = "fallback"
	}
	requiredCapable := systemSandboxRequiredCapable(mode, backend)
	capabilityLevel := strings.TrimSpace(backend.CapabilityLevel)
	if capabilityLevel == "" {
		capabilityLevel = systemSandboxCapabilityLevel(mode, backend)
	}
	payload := map[string]any{
		"mode":                     mode,
		"backend":                  backendName,
		"active":                   active,
		"required_capable":         requiredCapable,
		"capability_level":         capabilityLevel,
		"shell_network_isolation":  backend.Shell.Policy.NetworkIsolation,
		"worker_network_isolation": backend.Worker.Policy.NetworkIsolation,
		"fallback":                 fallback,
		"status":                   status,
	}
	if reason != "" {
		payload["fallback_reason"] = reason
	}
	return payload
}

func systemSandboxRequiredCapable(mode string, backend systemSandboxRuntimeBackend) bool {
	if backend.RequiredCapable {
		return true
	}
	if requiredSystemSandboxCapabilitiesSatisfied(backend) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(mode), systemSandboxModeRequired) &&
		requiredWindowsRequiredCapabilitiesSatisfied(backend)
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
		decision, err := execCtx.Approval(ApprovalRequest{
			ToolName: "run_shell",
			Command:  command,
			Reason:   reason,
		})
		if err != nil {
			return err
		}
		if !NormalizeApprovalDecision(decision).Approved() {
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

func validateRequiredWindowsShellCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("system sandbox mode required on windows only permits strict read-only commands")
	}
	if isPlanSafeCommand(command) {
		return nil
	}

	assessment := assessShellCommand(command)
	switch assessment.Risk {
	case shellRiskBlocked:
		return errors.New("system sandbox mode required on windows blocked command: " + assessment.Reason)
	default:
		reason := strings.TrimSpace(assessment.Reason)
		if reason == "" {
			reason = "command is not in strict read-only allowlist"
		}
		return errors.New("system sandbox mode required on windows only permits strict read-only shell commands: " + reason)
	}
}

func shellCommand(ctx context.Context, command string, execCtx *ExecutionContext) (*exec.Cmd, systemSandboxRuntimeBackend, string, error) {
	mode := normalizeSystemSandboxMode(execCtx)
	backend, err := resolveSystemSandboxRuntimeBackend(mode, runtime.GOOS, runShellLookPath)
	if err != nil {
		return nil, systemSandboxRuntimeBackend{}, mode, err
	}
	var cmd *exec.Cmd
	if backend.Enabled {
		execCommand := strings.TrimSpace(command)
		switch strings.TrimSpace(backend.Name) {
		case "darwin_sandbox_exec":
			allowNetwork := mode != systemSandboxModeRequired
			profile, err := buildDarwinSandboxProfile(execCtx, allowNetwork)
			if err != nil {
				return nil, backend, mode, err
			}
			cmd = exec.CommandContext(ctx, backend.Runner, buildDarwinSandboxShellArgs(profile, execCommand)...)
		case "windows_job_object":
			if mode == systemSandboxModeRequired {
				if err := validateRequiredWindowsShellCommand(execCommand); err != nil {
					return nil, backend, mode, err
				}
			}
			executable := resolveWindowsShellExecutable(exec.LookPath, os.Stat, os.Getenv)
			cmd = exec.CommandContext(ctx, executable, "-NoProfile", "-Command", command)
		default:
			if mode == systemSandboxModeRequired && runtime.GOOS == "linux" {
				wrapped, err := buildRequiredLinuxShellCommand(execCommand, execCtx)
				if err != nil {
					return nil, backend, mode, err
				}
				execCommand = wrapped
			}
			args := append(append([]string(nil), backend.Shell.ArgPrefix...), execCommand)
			cmd = exec.CommandContext(ctx, backend.Runner, args...)
		}
	} else if runtime.GOOS == "windows" {
		executable := resolveWindowsShellExecutable(exec.LookPath, os.Stat, os.Getenv)
		cmd = exec.CommandContext(ctx, executable, "-NoProfile", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}

	if backend.Enabled && mode == systemSandboxModeRequired {
		forced := map[string]string{}
		if execCtx != nil {
			workspace := strings.TrimSpace(execCtx.Workspace)
			if workspace != "" {
				forced["HOME"] = workspace
			}
		}
		forced["TMPDIR"] = "/tmp"
		cmd.Env = buildSandboxEnv(os.Environ(), sandboxEnvOptions{
			GOOS:          runtime.GOOS,
			RequiredMode:  true,
			DropSensitive: true,
			ForceSet:      forced,
		})
	}
	return cmd, backend, mode, nil
}

func withRequiredLinuxShellLimits(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return command
	}
	guards := []string{
		"ulimit -t 120 >/dev/null 2>&1 || true",
		"ulimit -f 1048576 >/dev/null 2>&1 || true",
		"ulimit -v 2097152 >/dev/null 2>&1 || true",
	}
	return strings.Join(append(guards, command), "; ")
}

type systemSandboxBackend struct {
	Enabled   bool
	Runner    string
	ArgPrefix []string
}

func normalizeSystemSandboxMode(execCtx *ExecutionContext) string {
	if execCtx == nil {
		return systemSandboxModeOff
	}
	switch strings.ToLower(strings.TrimSpace(execCtx.SystemSandboxMode)) {
	case "", systemSandboxModeOff:
		return systemSandboxModeOff
	case systemSandboxModeBestEffort:
		return systemSandboxModeBestEffort
	case systemSandboxModeRequired:
		return systemSandboxModeRequired
	default:
		return systemSandboxModeOff
	}
}

func resolveSystemSandboxBackend(mode, goos string, lookPath func(string) (string, error)) (systemSandboxBackend, error) {
	resolved, err := resolveSystemSandboxRuntimeBackend(mode, goos, lookPath)
	if err != nil {
		return systemSandboxBackend{}, err
	}
	if !resolved.Enabled {
		return systemSandboxBackend{}, nil
	}
	return systemSandboxBackend{
		Enabled:   true,
		Runner:    resolved.Runner,
		ArgPrefix: append([]string(nil), resolved.Shell.ArgPrefix...),
	}, nil
}

func linuxSystemSandboxNamespaceArgs() []string {
	return []string{
		"--user",
		"--map-root-user",
		"--mount",
		"--pid",
		"--fork",
		"--ipc",
		"--uts",
		"--net",
	}
}

func linuxSystemSandboxWorkerArgs() []string {
	return []string{
		"--user",
		"--map-root-user",
		"--mount",
		"--pid",
		"--fork",
		"--ipc",
		"--uts",
	}
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
