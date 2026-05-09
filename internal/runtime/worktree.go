package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeHandle carries metadata for an active worktree managed by WorktreeManager.
type WorktreeHandle struct {
	ID     string
	Path   string
	Branch string
	Commit string
}

// WorktreeRequest carries the parameters needed to create a worktree.
type WorktreeRequest struct {
	InvocationID  string
	WorkspaceRoot string
}

type worktreeOwnerMeta struct {
	WorktreeID string `json:"worktree_id"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	CreatedAt  string `json:"created_at"`
	State      string `json:"state"`
}

// WorktreeManager manages temporary git worktrees for subagent isolation.
type WorktreeManager struct {
	workspaceRoot string
	worktreesRoot string
	ownerDir      string
}

// NewWorktreeManager creates a WorktreeManager for the given workspace.
// Returns an error if the workspace is not a git repository.
func NewWorktreeManager(workspaceRoot string) (*WorktreeManager, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, fmt.Errorf("workspace is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("workspace %q is not a git repository", abs)
	}
	worktreesRoot := filepath.Join(abs, ".bytemind", "worktrees")
	ownerDir := worktreesRoot
	return &WorktreeManager{
		workspaceRoot: abs,
		worktreesRoot: worktreesRoot,
		ownerDir:      ownerDir,
	}, nil
}

// IsAvailable reports whether the manager is ready to create worktrees.
func (m *WorktreeManager) IsAvailable() bool {
	return m != nil
}

// Prepare creates a temporary git worktree and writes owner metadata.
func (m *WorktreeManager) Prepare(ctx context.Context, req WorktreeRequest) (*WorktreeHandle, error) {
	if m == nil {
		return nil, fmt.Errorf("worktree manager is not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	id := strings.TrimSpace(req.InvocationID)
	if id == "" {
		id = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	branch := "agent-" + id
	path := filepath.Join(m.worktreesRoot, "subagent-"+id)

	if err := os.MkdirAll(m.worktreesRoot, 0755); err != nil {
		return nil, fmt.Errorf("worktree: create root directory: %w", err)
	}

	// Record HEAD commit before creating the worktree.
	commitOut, err := exec.CommandContext(ctx, "git", "-C", m.workspaceRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("worktree: resolve HEAD: %w", err)
	}
	commit := strings.TrimSpace(string(commitOut))

	addCmd := exec.CommandContext(ctx, "git", "-C", m.workspaceRoot,
		"worktree", "add", "-b", branch, path, "HEAD")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worktree: git worktree add: %w\n%s", err, string(out))
	}

	meta := worktreeOwnerMeta{
		WorktreeID: id,
		Path:       path,
		Branch:     branch,
		Commit:     commit,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		State:      "active",
	}
	if err := m.writeOwnerMeta(id, meta); err != nil {
		// Rollback: remove the created worktree.
		_ = m.cleanupWorktree(ctx, path, branch)
		return nil, fmt.Errorf("worktree: write owner metadata: %w", err)
	}

	return &WorktreeHandle{
		ID:     id,
		Path:   path,
		Branch: branch,
		Commit: commit,
	}, nil
}

// Cleanup removes a worktree and its owner metadata. Idempotent.
func (m *WorktreeManager) Cleanup(ctx context.Context, h *WorktreeHandle) error {
	if m == nil || h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = m.cleanupWorktree(ctx, h.Path, h.Branch)
	_ = m.removeOwnerMeta(h.ID)
	return nil
}

func (m *WorktreeManager) cleanupWorktree(ctx context.Context, path, branch string) error {
	removeCmd := exec.CommandContext(ctx, "git", "-C", m.workspaceRoot,
		"worktree", "remove", path, "--force")
	_, _ = removeCmd.CombinedOutput() // ignore errors; path may already be removed

	branchCmd := exec.CommandContext(ctx, "git", "-C", m.workspaceRoot,
		"branch", "-D", branch)
	_, _ = branchCmd.CombinedOutput() // ignore errors; branch may already be gone

	return nil
}

// HasChanges detects whether a worktree has uncommitted modifications.
func (m *WorktreeManager) HasChanges(ctx context.Context, h *WorktreeHandle) (bool, error) {
	if m == nil || h == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	out, err := exec.CommandContext(ctx, "git", "-C", h.Path, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("worktree: git status: %w", err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func (m *WorktreeManager) ownerMetaPath(id string) string {
	return filepath.Join(m.ownerDir, id+".json")
}

func (m *WorktreeManager) writeOwnerMeta(id string, meta worktreeOwnerMeta) error {
	if err := os.MkdirAll(m.ownerDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.ownerMetaPath(id), data, 0644)
}

func (m *WorktreeManager) removeOwnerMeta(id string) error {
	err := os.Remove(m.ownerMetaPath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
