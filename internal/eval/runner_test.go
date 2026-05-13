package eval

import (
	"os"
	"os/exec"
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

func TestCheckCommandExitCodeHandling(t *testing.T) {
	// Test with nil exit code (no exit code check) - any command works
	// Use "git --help" which is available on most systems and exits 0
	ok, msg := CheckCommand("git --help", nil, ".")
	if !ok {
		t.Logf("git not available on this system: %s", msg)
		return
	}
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

func TestSmokeChecksSkipsCommandChecks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte(`package main`), 0o644)
	tasks := []EvalTask{{ID: "t1", Workspace: dir, Success: []Check{
		{FileContains: []FileContainsCheck{{Path: "test.go", Pattern: "package main"}}},
		{Command: "nonexistent_cmd_zzz"},
	}}}
	r := RunSmokeChecks(tasks)
	if len(r) != 1 || !r[0].Passed { t.Fatal("expected pass (command checks skipped in smoke)") }
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
	if len(LoadTasks(dir)) != 0 { t.Fatal("expected empty for LoadTasks") }
	_, errs := ValidateTasks(dir)
	if errs == 0 { t.Fatal("ValidateTasks should report error for bad YAML") }
}

func TestValidateTasksMixed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("id: t1\nname: Test\nworkspace: .\nprompt: test"), 0o644)
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{ invalid"), 0o644)
	tasks, errs := ValidateTasks(dir)
	if len(tasks) != 1 { t.Fatalf("expected 1 good task, got %d", len(tasks)) }
	if errs != 1 { t.Fatalf("expected 1 error, got %d", errs) }
}

func TestValidateTasksAllGood(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task.yaml"), []byte("id: t1\nname: Test\nworkspace: .\nprompt: test"), 0o644)
	tasks, errs := ValidateTasks(dir)
	if len(tasks) != 1 { t.Fatalf("expected 1 task, got %d", len(tasks)) }
	if errs != 0 { t.Fatalf("expected 0 errors, got %d", errs) }
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
	tasks := []EvalTask{{ID: "t", Workspace: ".", Prompt: "p", Success: []Check{{Command: "nonexistent_zzz"}}}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
}

func TestRunTasksWithOutputContains(t *testing.T) {
	tasks := []EvalTask{{
		ID: "t", Name: "Test", Workspace: ".", Prompt: "hello",
		Success: []Check{{OutputContains: []string{"world"}}},
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if r[0].Passed { t.Fatal("expected fail (no output from nonexistent binary)") }
}

// RunTasks negative eval paths — uses a git repo with pre-existing unstaged
// changes so gitDiffFiles returns real diffs even without a running agent.
func TestRunTasksNegativeReadOnly(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"file.go": "package main"})
	// Make an unstaged modification so gitDiffFiles returns a real diff
	os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main\n// modified"), 0o644)

	tasks := []EvalTask{{
		ID: "t1", Name: "read-only", Workspace: dir, Prompt: "explain",
		Negative: []NegCheck{{Type: "read_only"}},
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if r[0].Passed {
		t.Fatal("expected fail: read_only should detect modification")
	}
	if !strings.Contains(r[0].Failures[0], "negative[read_only]") {
		t.Fatalf("expected negative[read_only] failure, got: %v", r[0].Failures)
	}
}

func TestRunTasksNegativeForbiddenPaths(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"README.md": "# readme", "main.go": "package main"})
	// Modify README.md (should be forbidden)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# modified"), 0o644)

	tasks := []EvalTask{{
		ID: "t1", Name: "forbidden", Workspace: dir, Prompt: "fix",
		Negative: []NegCheck{{
			Type: "forbidden_paths",
			ForbiddenPaths: []string{"README.md"},
		}},
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if r[0].Passed {
		t.Fatal("expected fail: README.md is forbidden")
	}
	if !strings.Contains(r[0].Failures[0], "negative[forbidden_paths]") {
		t.Fatalf("expected negative[forbidden_paths], got: %v", r[0].Failures)
	}
}

func TestRunTasksNegativeMaxFilesChanged(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"a.go": "package a",
		"b.go": "package b",
		"c.go": "package c",
	})
	// Modify 2 files (max is 1)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n// mod"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n// mod"), 0o644)

	tasks := []EvalTask{{
		ID: "t1", Name: "maxfiles", Workspace: dir, Prompt: "fix",
		Negative: []NegCheck{{Type: "max_files_changed", MaxFilesChanged: 1}},
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if r[0].Passed {
		t.Fatal("expected fail: 2 files modified exceeds max 1")
	}
	if !strings.Contains(r[0].Failures[0], "negative[max_files_changed]") {
		t.Fatalf("expected negative[max_files_changed], got: %v", r[0].Failures)
	}
}

func TestRunTasksConstraintMaxFilesChanged(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"a.go": "package a", "b.go": "package b"})
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n// mod"), 0o644)

	tasks := []EvalTask{{
		ID: "t1", Name: "constraint", Workspace: dir, Prompt: "fix",
		Constraints: &Constraints{MaxFilesChanged: 0}, // 0 means no limit
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if !r[0].Passed {
		t.Fatalf("expected pass (constraint MaxFilesChanged=0 is no limit), got: %v", r[0].Failures)
	}
}

func TestRunTasksConstraintForbiddenPaths(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{
		"README.md": "# readme",
		"main.go":   "package main",
	})
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# modified"), 0o644)

	tasks := []EvalTask{{
		ID: "t1", Name: "constraint", Workspace: dir, Prompt: "fix",
		Constraints: &Constraints{ForbiddenPaths: []string{"README.md"}},
	}}
	r := RunTasks("/nonexistent_binary", tasks)
	if len(r) != 1 { t.Fatal("expected 1 result") }
	if r[0].Passed {
		t.Fatal("expected fail: README.md is forbidden by constraint")
	}
	if !strings.Contains(r[0].Failures[0], "constraint forbidden_paths") {
		t.Fatalf("expected constraint forbidden_paths, got: %v", r[0].Failures)
	}
}

func TestGitDiffFilesWithGitRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"file.txt": "hello"})
	files := gitDiffFiles(dir)
	if files == nil { t.Fatal("expected non-nil empty slice, got nil") }
	if len(files) != 0 { t.Fatalf("expected no modified files, got %v", files) }

	// Modify a file
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified"), 0o644)
	files = gitDiffFiles(dir)
	if len(files) != 1 || files[0] != "file.txt" {
		t.Fatalf("expected [file.txt], got %v", files)
	}
}

func TestListGitTrackedFilesWithGitRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"a.go": "package a", "b.go": "package b"})
	files := listGitTrackedFiles(dir)
	if len(files) != 2 { t.Fatalf("expected 2 files, got %v", files) }
}

func TestGitDiffFilesNonGitDir(t *testing.T) {
	dir := t.TempDir()
	files := gitDiffFiles(dir)
	if files != nil { t.Fatal("expected nil for non-git dir") }
}

func TestListGitTrackedFilesNonGitDir(t *testing.T) {
	dir := t.TempDir()
	files := listGitTrackedFiles(dir)
	if files != nil { t.Fatal("expected nil for non-git dir") }
}

func TestStringSetsEqual(t *testing.T) {
	if !stringSetsEqual([]string{"a", "b"}, []string{"b", "a"}) { t.Fatal("equal sets") }
	if stringSetsEqual([]string{"a"}, []string{"b"}) { t.Fatal("different sets") }
	if stringSetsEqual([]string{"a", "b"}, []string{"a"}) { t.Fatal("different lengths") }
}

func TestLoadTaskWithNegativeChecks(t *testing.T) {
	dir := t.TempDir()
	yaml := []byte(`
id: neg_test
name: Negative Eval Test
workspace: .
prompt: "Explain the code"
success:
  - output_contains:
      - "function"
negative:
  - type: read_only
    description: Should not modify files
  - type: forbidden_paths
    forbidden_paths:
      - "*.md"
  - type: max_files_changed
    max_files_changed: 1
`)
	os.WriteFile(filepath.Join(dir, "task.yaml"), yaml, 0o644)
	tasks := LoadTasks(dir)
	if len(tasks) != 1 { t.Fatal("expected 1 task") }
	task := tasks[0]
	if len(task.Negative) != 3 { t.Fatalf("expected 3 negative checks, got %d", len(task.Negative)) }
	if task.Negative[0].Type != "read_only" { t.Fatalf("expected read_only, got %s", task.Negative[0].Type) }
	if len(task.Negative[1].ForbiddenPaths) != 1 || task.Negative[1].ForbiddenPaths[0] != "*.md" {
		t.Fatalf("expected forbidden_paths [*.md], got %v", task.Negative[1].ForbiddenPaths)
	}
	if task.Negative[2].MaxFilesChanged != 1 { t.Fatalf("expected max_files_changed=1, got %d", task.Negative[2].MaxFilesChanged) }
}

func TestLoadTaskWithConstraints(t *testing.T) {
	dir := t.TempDir()
	yaml := []byte(`
id: const_test
name: Constraints Test
workspace: .
prompt: "Fix the bug"
success:
  - command: "go test ./..."
constraints:
  max_files_changed: 3
  forbidden_paths:
    - "README.md"
  require_test_run: true
`)
	os.WriteFile(filepath.Join(dir, "task.yaml"), yaml, 0o644)
	tasks := LoadTasks(dir)
	if len(tasks) != 1 { t.Fatal("expected 1 task") }
	task := tasks[0]
	if task.Constraints == nil { t.Fatal("expected constraints to be non-nil") }
	if task.Constraints.MaxFilesChanged != 3 { t.Fatalf("expected MaxFilesChanged=3, got %d", task.Constraints.MaxFilesChanged) }
	if len(task.Constraints.ForbiddenPaths) != 1 || task.Constraints.ForbiddenPaths[0] != "README.md" {
		t.Fatalf("expected forbidden_paths [README.md], got %v", task.Constraints.ForbiddenPaths)
	}
	if !task.Constraints.RequireTestRun { t.Fatal("expected RequireTestRun=true") }
}

// helpers

func initGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
	runCmd(t, dir, "git", "-c", "user.name=test", "-c", "user.email=test@test", "init")
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "initial")
}

func runCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd %s %v failed: %v\n%s", name, args, err, string(out))
	}
}
