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

func TestGitDiffToolNoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, true)

	tool := GitDiffTool{}
	raw, _ := json.Marshal(map[string]any{})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK      bool     `json:"ok"`
		Files   []string `json:"files"`
		Added   int      `json:"added"`
		Removed int      `json:"removed"`
		Summary string   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
	if len(out.Files) != 0 {
		t.Fatalf("expected no files changed, got %v", out.Files)
	}
	if !strings.Contains(out.Summary, "no changes") {
		t.Fatalf("expected 'no changes' summary, got %q", out.Summary)
	}
}

func TestGitDiffToolWithUnstagedChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, true)

	// Modify an existing tracked file
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := GitDiffTool{}
	raw, _ := json.Marshal(map[string]any{})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK      bool     `json:"ok"`
		Files   []string `json:"files"`
		Added   int      `json:"added"`
		Removed int      `json:"removed"`
		Diff    string   `json:"diff"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 file changed, got %d", len(out.Files))
	}
	if !strings.Contains(out.Files[0], "README.md") {
		t.Fatalf("expected README.md, got %v", out.Files)
	}
}

func TestGitDiffToolStagedChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, true)

	// Modify and stage
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# staged change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "add", ".").Run(); err != nil {
		t.Fatal(err)
	}

	tool := GitDiffTool{}
	raw, _ := json.Marshal(map[string]any{"staged": true})
	result, err := tool.Run(context.Background(), raw, &ExecutionContext{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		OK      bool     `json:"ok"`
		Files   []string `json:"files"`
		Added   int      `json:"added"`
		Removed int      `json:"removed"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 staged file, got %d", len(out.Files))
	}
}

func TestGitDiffToolNotARepo(t *testing.T) {
	dir := t.TempDir()

	tool := GitDiffTool{}
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
}
