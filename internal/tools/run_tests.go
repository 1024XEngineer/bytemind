package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type RunTestsTool struct{}

func (RunTestsTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "run_tests",
			Description: "Auto-detect and run tests for the project in the workspace. Supports Go, Node, Python, Rust, and other common test frameworks.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Optional subdirectory to run tests in. Defaults to workspace root.",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Explicit test command override (e.g. 'go test ./...', 'npm test'). If empty, auto-detected from project files.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds. Defaults to 120.",
					},
				},
			},
		},
	}
}

func (RunTestsTool) Spec() ToolSpec {
	return ToolSpec{
		ConcurrencySafe: false,
		SafetyClass:     SafetyClassSensitive,
		DefaultTimeoutS: 120,
		MaxTimeoutS:     600,
		MaxResultChars:  128 * 1024,
	}
}

func (RunTestsTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	dir := execCtx.Workspace
	var args struct {
		Path           string `json:"path"`
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Path) != "" {
		resolved, err := resolvePath(execCtx.Workspace, args.Path)
		if err == nil {
			dir = resolved
		}
	}

	command := strings.TrimSpace(args.Command)
	if command == "" {
		detected, ok := detectTestCommand(dir)
		if !ok {
			return toJSON(map[string]any{
				"ok":         false,
				"error":      "could not auto-detect test command",
				"details":    "No recognized test framework found. Supported: go.mod, package.json, Cargo.toml, pytest.ini, setup.py, Makefile with test target.",
				"detected":   false,
				"exit_code":  -1,
			})
		}
		command = detected
	}

	timeout := 120 * time.Second
	if args.TimeoutSeconds > 0 {
		if args.TimeoutSeconds > 600 {
			args.TimeoutSeconds = 600
		}
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(runCtx, detectShell(), detectShellArgs(command)...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	exitCode := 0
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return toJSON(map[string]any{
				"ok":        false,
				"error":     "test run timed out",
				"timeout_s": int(timeout.Seconds()),
				"command":   command,
			})
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return toJSON(map[string]any{
				"ok":      false,
				"error":   "failed to run tests",
				"details": err.Error(),
				"command": command,
			})
		}
	}

	outText := stdout.String()
	errText := stderr.String()
	combined := outText + errText

	passed := extractTestCount(combined, "ok")
	failed := extractTestCount(combined, "FAIL")
	skipped := extractSkippedCount(combined)

	if failed == 0 && exitCode != 0 {
		if strings.Contains(combined, "FAIL") {
			failed = countOccurrences(combined, "--- FAIL:")
		}
	}

	truncated := false
	display := outText
	if len(outText) > 64*1024 {
		display = outText[:64*1024] + "\n... (output truncated at 64KB)"
		truncated = true
	}

	summary := fmt.Sprintf("Tests: passed=%d failed=%d skipped=%d (%.1fs)", passed, failed, skipped, elapsed.Seconds())
	if exitCode != 0 {
		summary = fmt.Sprintf("Tests FAILED: passed=%d failed=%d skipped=%d (%.1fs)", passed, failed, skipped, elapsed.Seconds())
	}
	if exitCode == 0 && passed == 0 && failed == 0 {
		summary = fmt.Sprintf("No tests found (%.1fs)", elapsed.Seconds())
	}

	return toJSON(map[string]any{
		"ok":         exitCode == 0,
		"passed":     passed,
		"failed":     failed,
		"skipped":    skipped,
		"exit_code":  exitCode,
		"elapsed_s":  elapsed.Seconds(),
		"command":    command,
		"stdout":     display,
		"stderr":     errText,
		"summary":    summary,
		"truncated":  truncated,
	})
}

func detectTestCommand(dir string) (string, bool) {
	hasFile := func(name string) bool {
		_, err := filepath.Glob(filepath.Join(dir, name))
		return err == nil
	}
	fileExists := func(name string) bool {
		info, err := filepath.Glob(filepath.Join(dir, name))
		if err != nil || len(info) == 0 {
			_, err2 := filepath.Glob(filepath.Join(filepath.Dir(dir), name))
			return err2 == nil
		}
		return true
	}

	if _, err := exec.LookPath("go"); err == nil {
		if hasFile("go.mod") {
			return "go test ./...", true
		}
	}
	if _, err := exec.LookPath("npm"); err == nil {
		if hasFile("package.json") {
			return "npm test 2>&1", true
		}
	}
	if _, err := exec.LookPath("cargo"); err == nil {
		if hasFile("Cargo.toml") {
			return "cargo test 2>&1", true
		}
	}
	if _, err := exec.LookPath("python3"); err == nil {
		if hasFile("pytest.ini") || hasFile("setup.py") || hasFile("pyproject.toml") {
			return "python3 -m pytest 2>&1", true
		}
	}
	if _, err := exec.LookPath("make"); err == nil {
		if hasFile("Makefile") {
			return "make test 2>&1", true
		}
	}

	if fileExists("go.mod") {
		if _, err := exec.LookPath("go"); err == nil {
			return "go test ./...", true
		}
	}
	if fileExists("package.json") {
		if _, err := exec.LookPath("npm"); err == nil {
			return "npm test 2>&1", true
		}
	}

	return "", false
}

func detectShell() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	return "cmd"
}

func detectShellArgs(command string) []string {
	shell := detectShell()
	if shell == "cmd" {
		return []string{"/C", command}
	}
	return []string{"-c", command}
}

func extractTestCount(output, prefix string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			count++
		}
	}
	return count
}

func extractSkippedCount(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "--- SKIP:") {
			count++
		}
	}
	return count
}

func countOccurrences(s, substr string) int {
	return strings.Count(s, substr)
}
