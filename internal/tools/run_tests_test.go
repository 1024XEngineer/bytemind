package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTestsToolGoProject(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}
	if detectShell() == "cmd" {
		t.Skip("run_tests uses shell wrapper; cmd.exe may not resolve Go PATH correctly")
	}
	dir := t.TempDir()
	initGoProject(t, dir)

	tool := RunTestsTool{}
	raw, _ := json.Marshal(map[string]any{
		"command": "go test ./...",
	})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatalf("expected tests to pass, got exit_code=%d", out.ExitCode)
	}
}

func TestRunTestsToolCustomCommand(t *testing.T) {
	if detectShell() == "cmd" {
		t.Skip("run_tests uses shell wrapper; cmd.exe may not resolve Go PATH correctly")
	}
	dir := t.TempDir()

	tool := RunTestsTool{}
	raw, _ := json.Marshal(map[string]any{
		"command": "go version",
	})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Skip("go version command failed:", err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatalf("expected 'go version' to succeed, got exit_code=%d", out.ExitCode)
	}
}

func TestDetectTestCommandGoMod(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}
	dir := t.TempDir()

	cmd, ok := detectTestCommand(dir)
	if ok {
		t.Fatalf("expected no detection in empty dir, got %q", cmd)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, ok = detectTestCommand(dir)
	if !ok {
		t.Fatal("expected detection with go.mod")
	}
	if !strings.Contains(cmd, "go test") {
		t.Fatalf("expected 'go test' in command, got %q", cmd)
	}
}

func TestDetectTestCommandEmptyDir(t *testing.T) {
	dir := t.TempDir()
	cmd, ok := detectTestCommand(dir)
	if ok {
		t.Fatalf("expected no detection in empty dir, got %q", cmd)
	}
	_ = cmd
}

func TestExtractCounts(t *testing.T) {
	output := "ok package1 0.001s\nFAIL package2 0.002s\n--- FAIL: TestSomething (0.00s)\nok package3 0.001s\n"
	passed := extractTestCount(output, "ok")
	failed := extractTestCount(output, "FAIL")
	if passed != 2 {
		t.Fatalf("expected 2 passed, got %d", passed)
	}
	if failed != 1 {
		t.Fatalf("expected 1 failed (from FAIL prefix), got %d", failed)
	}
}

func TestParseShellArgs(t *testing.T) {
	shell := detectShell()
	args := detectShellArgs("echo hello")
	if shell == "cmd" {
		if len(args) != 2 || args[0] != "/C" || args[1] != "echo hello" {
			t.Fatalf("unexpected cmd args: %v", args)
		}
	} else {
		if len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
			t.Fatalf("unexpected sh args: %v", args)
		}
	}
}

func initGoProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pass_test.go"), []byte("package test\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\nfunc TestAlsoPass(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
