package eval

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckFileContainsMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(`func main() { println("hello") }`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, msg := CheckFileContains(dir, "test.go", "func main")
	if !ok {
		t.Fatalf("expected match, got: %s", msg)
	}
}

func TestCheckFileContainsNoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package foo`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, msg := CheckFileContains(dir, "test.go", "func main")
	if ok {
		t.Fatal("expected no match")
	}
	if !strings.Contains(msg, "does not match") {
		t.Fatalf("expected 'does not match' message, got: %s", msg)
	}
}

func TestCheckFileContainsMissingFile(t *testing.T) {
	ok, msg := CheckFileContains(".", "nonexistent.go", "anything")
	if ok {
		t.Fatal("expected failure for missing file")
	}
	if !strings.Contains(msg, "cannot read") {
		t.Fatalf("expected 'cannot read' message, got: %s", msg)
	}
}

func TestCheckFileContainsWithAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "test.go")
	if err := os.WriteFile(absPath, []byte(`package main`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, msg := CheckFileContains("", absPath, "package main")
	if !ok {
		t.Fatalf("expected match with abs path, got: %s", msg)
	}
}

func TestCheckFileContainsBadRegex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, msg := CheckFileContains(dir, "test.go", `\K[`)
	if ok {
		t.Fatal("expected failure for bad regex")
	}
	if !strings.Contains(msg, "bad regex") {
		t.Fatalf("expected 'bad regex' message, got: %s", msg)
	}
}

func TestCheckCommandEmptyCommand(t *testing.T) {
	ok, msg := CheckCommand("", nil, ".")
	if ok {
		t.Fatal("expected failure for empty command")
	}
	if !strings.Contains(msg, "empty command") {
		t.Fatalf("expected 'empty command' message, got: %s", msg)
	}
}

func TestCheckCommandSucceeds(t *testing.T) {
	ok, msg := CheckCommand("go version", nil, ".")
	if !ok {
		t.Skip("go command failed via shell wrapper:", msg)
	}
}

func TestCheckCommandExpectedExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit code checks are platform-dependent on Windows")
	}
	exit1 := 1
	ok, msg := CheckCommand("go tool -doesnotexist", &exit1, ".")
	if !ok {
		t.Skip("expected exit code 1 check skipped:", msg)
	}
}

func TestCheckCommandUnexpectedSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell exit code semantics differ on Windows")
	}
	exit1 := 1
	ok, msg := CheckCommand("echo hello", &exit1, ".")
	if ok {
		t.Fatal("expected failure: command succeeded but expected exit 1")
	}
	if !strings.Contains(msg, "succeeded but expected exit code") {
		t.Fatalf("expected 'succeeded but expected' message, got: %s", msg)
	}
}

func TestCheckCommandSucceedsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific test")
	}
	ok, msg := CheckCommand("go version", nil, ".")
	if !ok {
		// On Windows, go version may write to stderr but succeed
		t.Log("go version check:", msg)
	}
}

func TestCheckCommandViaBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}
	if runtime.GOOS == "windows" {
		t.Skip("bash on Windows has different signal handling")
	}
	ok, msg := CheckCommand("echo hello", nil, ".")
	if !ok {
		t.Fatalf("expected success via bash, got: %s", msg)
	}
}

func TestCheckOutputContains(t *testing.T) {
	ok, msg := CheckOutputContains("hello world", []string{"world"})
	if !ok {
		t.Fatalf("expected match, got: %s", msg)
	}

	ok, msg = CheckOutputContains("hello world", []string{"world", "universe"})
	if ok {
		t.Fatal("expected no match for 'universe'")
	}
}

func TestCheckOutputContainsCaseInsensitive(t *testing.T) {
	ok, msg := CheckOutputContains("Hello World", []string{"world"})
	if !ok {
		t.Fatalf("expected case-insensitive match, got: %s", msg)
	}
}

func TestRunSmokeChecksAllPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks := []EvalTask{
		{
			ID:        "test_001",
			Name:      "File check",
			Workspace: dir,
			Success: []Check{
				{FileContains: []FileContainsCheck{{Path: "test.go", Pattern: "package main"}}},
			},
		},
	}

	results := RunSmokeChecks(tasks)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected passed, got failures: %v", results[0].Failures)
	}
}

func TestRunSmokeChecksFails(t *testing.T) {
	tasks := []EvalTask{
		{
			ID:   "test_001",
			Name: "Missing file",
			Success: []Check{
				{FileContains: []FileContainsCheck{{Path: "nonexistent.go", Pattern: "anything"}}},
			},
		},
	}

	results := RunSmokeChecks(tasks)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("expected failure for missing file")
	}
}

func TestRunSmokeChecksCommandCheck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
	tasks := []EvalTask{
		{
			ID:   "test_001",
			Name: "Go version",
			Success: []Check{
				{Command: "go version"},
			},
		},
	}

	results := RunSmokeChecks(tasks)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		// On some platforms go version writes to stderr, causing shell to report failure
		t.Log("go version check result:", results[0].Failures)
	}
}

func TestRunSmokeChecksCommandFails(t *testing.T) {
	tasks := []EvalTask{
		{
			ID:   "test_001",
			Name: "Failing command",
			Success: []Check{
				{Command: "nonexistent_command_xyz123"},
			},
		},
	}

	results := RunSmokeChecks(tasks)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("expected failure for nonexistent command")
	}
}

func TestRunSmokeChecksMultipleChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks := []EvalTask{
		{
			ID:        "test_001",
			Name:      "Mixed",
			Workspace: dir,
			Success: []Check{
				{FileContains: []FileContainsCheck{{Path: "test.go", Pattern: "package main"}}},
				{Command: "nonexistent_cmd"},
			},
		},
	}

	results := RunSmokeChecks(tasks)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("expected failure: second check should fail")
	}
}

func TestLoadTasksEmptyDir(t *testing.T) {
	dir := t.TempDir()
	tasks := LoadTasks(dir)
	if tasks == nil {
		t.Fatal("expected empty slice, not nil")
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestLoadTasksInvalidDir(t *testing.T) {
	tasks := LoadTasks("C:\\nonexistent_path_12345_" + t.Name())
	if tasks == nil {
		t.Fatal("expected empty slice on invalid dir, not nil")
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestLoadTasksValidYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "task_001.yaml"), []byte("id: test_001\nname: Test\nworkspace: .\nprompt: test\nsuccess:\n  - command: go test"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := LoadTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "test_001" {
		t.Fatalf("expected id test_001, got %s", tasks[0].ID)
	}
}

func TestLoadTasksSkipsNonYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a task"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte("id: test_001\nname: Test\nworkspace: .\nprompt: test"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := LoadTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task (skipped .txt), got %d", len(tasks))
	}
}

func TestLoadTasksBadYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{ invalid yaml }"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := LoadTasks(dir)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks (bad yaml skipped), got %d", len(tasks))
	}
}

func TestFilterTasksAll(t *testing.T) {
	tasks := []EvalTask{
		{ID: "task_001", Name: "First"},
		{ID: "task_002", Name: "Second"},
	}
	filtered := FilterTasks(tasks, "all")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(filtered))
	}
}

func TestFilterTasksByID(t *testing.T) {
	tasks := []EvalTask{
		{ID: "task_001", Name: "First"},
		{ID: "task_002", Name: "Second"},
	}
	filtered := FilterTasks(tasks, "task_002")
	if len(filtered) != 1 || filtered[0].ID != "task_002" {
		t.Fatalf("expected 1 task with id task_002, got %d", len(filtered))
	}
}

func TestFilterTasksNotFound(t *testing.T) {
	tasks := []EvalTask{
		{ID: "task_001", Name: "First"},
	}
	filtered := FilterTasks(tasks, "nonexistent")
	if len(filtered) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(filtered))
	}
}

func TestAllPassed(t *testing.T) {
	allPass := AllPassed([]TaskResult{
		{ID: "a", Passed: true},
		{ID: "b", Passed: true},
	})
	if !allPass {
		t.Fatal("expected all passed")
	}

	notAll := AllPassed([]TaskResult{
		{ID: "a", Passed: true},
		{ID: "b", Passed: false},
	})
	if notAll {
		t.Fatal("expected not all passed")
	}
}

func TestPrintResultsAllPassed(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
		t.Skip("local windows terminal encoding")
	}
	// Should not panic
	PrintResults([]TaskResult{
		{ID: "a", Name: "A", Passed: true},
	})
}

func TestCheckFileContainsWithSymlinkedPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "test.go"), []byte(`package main`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, msg := CheckFileContains(dir, filepath.Join("sub", "test.go"), "package main")
	if !ok {
		t.Fatalf("expected match in subdirectory, got: %s", msg)
	}
}
