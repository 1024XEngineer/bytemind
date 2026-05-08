package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWorktreeManager_GitRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wm.IsAvailable() {
		t.Fatal("expected manager to be available")
	}
}

func TestNewWorktreeManager_NonGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := NewWorktreeManager(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestNewWorktreeManager_EmptyWorkspace(t *testing.T) {
	_, err := NewWorktreeManager("")
	if err == nil {
		t.Fatal("expected error for empty workspace")
	}
}

func TestWorktreeManager_Prepare(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "test-agent-001",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if handle == nil {
		t.Fatal("expected non-nil handle")
	}
	if handle.Path == "" {
		t.Fatal("expected non-empty path")
	}
	if handle.Branch == "" {
		t.Fatal("expected non-empty branch")
	}
	if handle.Commit == "" {
		t.Fatal("expected non-empty commit")
	}

	// Verify worktree path exists and has .git pointer.
	worktreeGit := filepath.Join(handle.Path, ".git")
	if info, err := os.Stat(worktreeGit); err != nil || info == nil {
		t.Fatalf("expected .git file/dir in worktree at %s", worktreeGit)
	}

	// Cleanup.
	if err := wm.Cleanup(context.Background(), handle); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify worktree is removed.
	if _, err := os.Stat(handle.Path); !os.IsNotExist(err) {
		t.Fatal("expected worktree path to be removed after cleanup")
	}
}

func TestWorktreeManager_HasChanges_NoChanges(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "test-agent-002",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer wm.Cleanup(context.Background(), handle)

	changed, err := wm.HasChanges(context.Background(), handle)
	if err != nil {
		t.Fatalf("HasChanges failed: %v", err)
	}
	if changed {
		t.Fatal("expected no changes in fresh worktree")
	}
}

func TestWorktreeManager_HasChanges_WithChanges(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "test-agent-003",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer wm.Cleanup(context.Background(), handle)

	// Modify a file in the worktree.
	writeFile(t, handle.Path, "README.md", "# modified")
	changed, err := wm.HasChanges(context.Background(), handle)
	if err != nil {
		t.Fatalf("HasChanges failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changes after file modification")
	}
}

func TestWorktreeManager_Cleanup_Idempotent(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "test-agent-004",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// First cleanup.
	if err := wm.Cleanup(context.Background(), handle); err != nil {
		t.Fatalf("first Cleanup failed: %v", err)
	}
	// Second cleanup should not error.
	if err := wm.Cleanup(context.Background(), handle); err != nil {
		t.Fatalf("second Cleanup failed: %v", err)
	}
}

func TestWorktreeManager_IsAvailable(t *testing.T) {
	var nilManager *WorktreeManager
	if nilManager.IsAvailable() {
		t.Fatal("nil manager should not be available")
	}

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wm.IsAvailable() {
		t.Fatal("expected manager to be available")
	}
}

func TestWorktreeManager_Prepare_MultipleWorktrees(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# test")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle1, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "agent-001",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare 1 failed: %v", err)
	}
	defer wm.Cleanup(context.Background(), handle1)

	handle2, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "agent-002",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare 2 failed: %v", err)
	}
	defer wm.Cleanup(context.Background(), handle2)

	// Verify they are different paths.
	if handle1.Path == handle2.Path {
		t.Fatal("expected different worktree paths")
	}

	// Modify handle1 and verify handle2 is unchanged.
	writeFile(t, handle1.Path, "README.md", "# changed in wt1")
	changed1, _ := wm.HasChanges(context.Background(), handle1)
	changed2, _ := wm.HasChanges(context.Background(), handle2)
	if !changed1 {
		t.Fatal("expected changes in worktree 1")
	}
	if changed2 {
		t.Fatal("expected no changes in worktree 2")
	}
}

func TestWorktreeManager_WritableRoots_Isolation(t *testing.T) {
	// Verify that writes in the worktree don't affect the parent workspace.
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, dir, "README.md", "# original")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")

	wm, err := NewWorktreeManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handle, err := wm.Prepare(context.Background(), WorktreeRequest{
		InvocationID:  "agent-isolation",
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer wm.Cleanup(context.Background(), handle)

	// Write to the worktree.
	writeFile(t, handle.Path, "README.md", "# modified in worktree")

	// Verify parent is unchanged.
	parentContent := readFile(t, dir, "README.md")
	if parentContent != "# original" {
		t.Fatalf("parent workspace was modified: %q", parentContent)
	}

	// Verify worktree has the change.
	wtContent := readFile(t, handle.Path, "README.md")
	if wtContent != "# modified in worktree" {
		t.Fatalf("worktree content mismatch: %q", wtContent)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	return strings.TrimSpace(string(data))
}
