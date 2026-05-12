package eval

import (
	"os"
	"path/filepath"
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

func TestCheckCommandEmptyCommand(t *testing.T) {
	ok, msg := CheckCommand("", nil, ".")
	if ok {
		t.Fatal("expected failure for empty command")
	}
	if !strings.Contains(msg, "empty command") {
		t.Fatalf("expected 'empty command' message, got: %s", msg)
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
	tasks := LoadTasks("C:\\nonexistent_path_12345")
	if tasks == nil {
		t.Fatal("expected empty slice on invalid dir, not nil")
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
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
