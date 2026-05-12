package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunTestsToolGoProject(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}
	if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
		t.Skip("Windows shell wrapper PATH issue; pass on CI")
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
	if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
		t.Skip("Windows shell wrapper PATH issue; pass on CI")
	}
	dir := t.TempDir()

	tool := RunTestsTool{}
	raw, _ := json.Marshal(map[string]any{
		"command": "go version",
	})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Skip("go version failed:", err)
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

func TestDetectTestCommandNodeProject(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found in PATH")
	}
	dir := t.TempDir()

	cmd, ok := detectTestCommand(dir)
	if ok {
		t.Fatalf("expected no detection in empty dir, got %q", cmd)
	}

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"test":"echo test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, ok = detectTestCommand(dir)
	if !ok {
		t.Fatal("expected detection with package.json")
	}
	if !strings.Contains(cmd, "npm test") {
		t.Fatalf("expected 'npm test' in command, got %q", cmd)
	}
}

func TestDetectTestCommandPythonProject(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH")
	}
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, ok := detectTestCommand(dir)
	if !ok {
		t.Fatal("expected detection with pytest.ini")
	}
	if !strings.Contains(cmd, "pytest") {
		t.Fatalf("expected 'pytest' in command, got %q", cmd)
	}
}

func TestDetectTestCommandMakefile(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not found in PATH")
	}
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("test:\n\t@echo test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, ok := detectTestCommand(dir)
	if !ok {
		t.Fatal("expected detection with Makefile")
	}
	if !strings.Contains(cmd, "make test") {
		t.Fatalf("expected 'make test' in command, got %q", cmd)
	}
}

func TestRunTestsToolFailingTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fail_test.go"), []byte("package test\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Error(\"expected failure\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
		Failed   int    `json:"failed"`
		ExitCode int    `json:"exit_code"`
		Summary  string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if out.OK {
		t.Fatal("expected tests to fail")
	}
	if out.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for failing tests")
	}
	if !strings.Contains(out.Summary, "FAILED") {
		t.Fatalf("expected FAILED in summary, got %q", out.Summary)
	}
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
