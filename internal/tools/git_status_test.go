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

func TestGitStatusToolClean(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, true)

	tool := GitStatusTool{}
	raw, _ := json.Marshal(map[string]any{})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Branch  string `json:"branch"`
		Total   int    `json:"total"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
	if out.Total != 0 {
		t.Fatalf("expected total=0 on clean repo, got %d", out.Total)
	}
}

func TestGitStatusToolWithUntracked(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, true)

	os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "another.txt"), []byte("hello"), 0o644)

	tool := GitStatusTool{}
	raw, _ := json.Marshal(map[string]any{})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK        bool     `json:"ok"`
		Untracked []string `json:"untracked"`
		Total     int      `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
	if out.Total != 2 {
		t.Fatalf("expected total=2, got %d", out.Total)
	}
	if len(out.Untracked) != 2 {
		t.Fatalf("expected 2 untracked, got %d", len(out.Untracked))
	}
}

func TestGitStatusToolNotARepo(t *testing.T) {
	dir := t.TempDir()

	tool := GitStatusTool{}
	raw, _ := json.Marshal(map[string]any{})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if out.OK {
		t.Fatal("expected ok=false for non-git dir")
	}
	if !strings.Contains(out.Error, "not a git repository") {
		t.Fatalf("expected git error, got %q", out.Error)
	}
}

func initGitRepo(t *testing.T, dir string, addCommit bool) {
	t.Helper()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.email", "test@test").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.name", "test").Run(); err != nil {
		t.Fatal(err)
	}
	if addCommit {
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("git", "-C", dir, "add", ".").Run(); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("git", "-C", dir, "commit", "-m", "init").Run(); err != nil {
			t.Fatal(err)
		}
	}
}
