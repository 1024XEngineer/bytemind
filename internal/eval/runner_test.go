package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFileContainsMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`func main() {}`), 0o644)
	ok, msg := CheckFileContains(dir, "test.go", "func main")
	if !ok { t.Fatalf("expected match, got: %s", msg) }
}

func TestCheckFileContainsNoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package foo`), 0o644)
	ok, msg := CheckFileContains(dir, "test.go", "func main")
	if ok { t.Fatal("expected no match") }
	if !strings.Contains(msg, "does not match") { t.Fatal("bad msg:", msg) }
}

func TestCheckFileContainsMissingFile(t *testing.T) {
	ok, msg := CheckFileContains(".", "nonexistent.go", "anything")
	if ok { t.Fatal("expected failure") }
	if !strings.Contains(msg, "cannot read") { t.Fatal("bad msg:", msg) }
}

func TestCheckFileContainsWithAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "test.go")
	os.WriteFile(abs, []byte(`package main`), 0o644)
	ok, msg := CheckFileContains("", abs, "package main")
	if !ok { t.Fatalf("abs path match fail: %s", msg) }
}

func TestCheckFileContainsBadRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644)
	ok, msg := CheckFileContains(dir, "test.go", `\K[`)
	if ok { t.Fatal("expected bad regex failure") }
	if !strings.Contains(msg, "bad regex") { t.Fatal("bad msg:", msg) }
}

func TestCheckCommandEmpty(t *testing.T) {
	ok, msg := CheckCommand("", nil, ".")
	if ok { t.Fatal("expected failure") }
	if !strings.Contains(msg, "empty command") { t.Fatal("bad msg:", msg) }
}

func TestCheckOutputContains(t *testing.T) {
	ok, _ := CheckOutputContains("hello world", []string{"world"})
	if !ok { t.Fatal("expected match") }
	ok, _ = CheckOutputContains("hello world", []string{"world", "universe"})
	if ok { t.Fatal("expected no match") }
}

func TestCheckOutputContainsCaseInsensitive(t *testing.T) {
	ok, _ := CheckOutputContains("Hello World", []string{"world"})
	if !ok { t.Fatal("expected case-insensitive match") }
}

func TestSmokeChecksFileCheckPass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644)
	tasks := []EvalTask{{ID: "t1", Workspace: dir, Success: []Check{{FileContains: []FileContainsCheck{{Path: "test.go", Pattern: "package main"}}}}}}
	r := RunSmokeChecks(tasks)
	if len(r) != 1 || !r[0].Passed { t.Fatal("expected pass") }
}

func TestSmokeChecksFileCheckFail(t *testing.T) {
	tasks := []EvalTask{{ID: "t1", Success: []Check{{FileContains: []FileContainsCheck{{Path: "nonexistent.go", Pattern: "anything"}}}}}}
	r := RunSmokeChecks(tasks)
	if len(r) != 1 || r[0].Passed { t.Fatal("expected fail") }
}

func TestSmokeChecksMultiple(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644)
	tasks := []EvalTask{{ID: "t1", Workspace: dir, Success: []Check{
		{FileContains: []FileContainsCheck{{Path: "test.go", Pattern: "package main"}}},
		{Command: "nonexistent_cmd_zzz"},
	}}}
	r := RunSmokeChecks(tasks)
	if len(r) != 1 || r[0].Passed { t.Fatal("expected fail on second check") }
}

func TestLoadTasksEmptyDir(t *testing.T) {
	tasks := LoadTasks(t.TempDir())
	if tasks == nil || len(tasks) != 0 { t.Fatal("expected empty") }
}

func TestLoadTasksInvalidDir(t *testing.T) {
	tasks := LoadTasks("C:\\nx_" + t.Name())
	if tasks == nil || len(tasks) != 0 { t.Fatal("expected empty") }
}

func TestLoadTasksValidYaml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task_001.yaml"), []byte("id: t1\nname: Test\nworkspace: .\nprompt: test\nsuccess:\n  - command: go test"), 0o644)
	tasks := LoadTasks(dir)
	if len(tasks) != 1 || tasks[0].ID != "t1" { t.Fatal("expected 1 task t1") }
}

func TestLoadTasksSkipsNonYaml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a task"), 0o644)
	os.WriteFile(filepath.Join(dir, "task.yaml"), []byte("id: t1\nname: Test\nworkspace: .\nprompt: test"), 0o644)
	if len(LoadTasks(dir)) != 1 { t.Fatal("expected 1") }
}

func TestLoadTasksBadYaml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{ invalid"), 0o644)
	if len(LoadTasks(dir)) != 0 { t.Fatal("expected 0") }
}

func TestFilterTasks(t *testing.T) {
	tasks := []EvalTask{{ID: "a"}, {ID: "b"}}
	if len(FilterTasks(tasks, "all")) != 2 { t.Fatal("all") }
	if len(FilterTasks(tasks, "b")) != 1 { t.Fatal("by id") }
	if len(FilterTasks(tasks, "x")) != 0 { t.Fatal("not found") }
}

func TestAllPassed(t *testing.T) {
	if !AllPassed([]TaskResult{{Passed: true}}) { t.Fatal("all passed") }
	if AllPassed([]TaskResult{{Passed: false}}) { t.Fatal("not all passed") }
}

func TestPrintResults(t *testing.T) {
	PrintResults([]TaskResult{{ID: "a", Name: "A", Passed: true}})
	PrintResults([]TaskResult{{ID: "b", Name: "B", Passed: false, Failures: []string{"f1"}}})
}

func TestRunTasksCoverage(t *testing.T) {
	// Call RunTasks to exercise all code paths without external dependencies
	tasks := []EvalTask{{ID: "t", Workspace: ".", Prompt: "p", Success: []Check{{Command: "nonexistent_zzz"}}}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
}
