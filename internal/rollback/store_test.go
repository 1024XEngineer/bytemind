package rollback

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRollsBackCommittedUpdate(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	mustWriteRollbackTestFile(t, path, "old\n")

	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	op, err := store.Begin(context.Background(), BeginOptions{
		Workspace: workspace,
		ToolName:  "write_file",
		TraceID:   "trace-test",
	}, []FileTarget{{
		Path:    "file.txt",
		AbsPath: path,
		OpType:  OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}

	mustWriteRollbackTestFile(t, path, "new\n")
	if err := store.Commit(context.Background(), op); err != nil {
		t.Fatal(err)
	}

	rolledBack, err := store.Rollback(context.Background(), workspace, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != StatusRolledBack {
		t.Fatalf("expected rolled_back status, got %s", rolledBack.Status)
	}
	if got := mustReadRollbackTestFile(t, path); got != "old\n" {
		t.Fatalf("expected old content restored, got %q", got)
	}
}

func TestStoreRollbackBlocksOnConflict(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	mustWriteRollbackTestFile(t, path, "old\n")

	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	op, err := store.Begin(context.Background(), BeginOptions{
		Workspace: workspace,
		ToolName:  "replace_in_file",
		TraceID:   "trace-test",
	}, []FileTarget{{
		Path:    "file.txt",
		AbsPath: path,
		OpType:  OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteRollbackTestFile(t, path, "new\n")
	if err := store.Commit(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	mustWriteRollbackTestFile(t, path, "user edit\n")

	_, err = store.Rollback(context.Background(), workspace, op.OperationID)
	if err == nil || !strings.Contains(err.Error(), "changed after operation") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if got := mustReadRollbackTestFile(t, path); got != "user edit\n" {
		t.Fatalf("expected conflict to leave user edit intact, got %q", got)
	}
}

func TestStoreRollsBackAddDeleteAndMove(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}

	addPath := filepath.Join(workspace, "added.txt")
	addOp, err := store.Begin(context.Background(), BeginOptions{Workspace: workspace, ToolName: "apply_patch", TraceID: "trace-add"}, []FileTarget{{
		Path:    "added.txt",
		AbsPath: addPath,
		OpType:  OpTypeAdd,
	}})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteRollbackTestFile(t, addPath, "new\n")
	if err := store.Commit(context.Background(), addOp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, addOp.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(addPath); !os.IsNotExist(err) {
		t.Fatalf("expected added file removed, got %v", err)
	}

	deletePath := filepath.Join(workspace, "delete.txt")
	mustWriteRollbackTestFile(t, deletePath, "before delete\n")
	deleteOp, err := store.Begin(context.Background(), BeginOptions{Workspace: workspace, ToolName: "apply_patch", TraceID: "trace-delete"}, []FileTarget{{
		Path:    "delete.txt",
		AbsPath: deletePath,
		OpType:  OpTypeDelete,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deletePath); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(context.Background(), deleteOp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, deleteOp.OperationID); err != nil {
		t.Fatal(err)
	}
	if got := mustReadRollbackTestFile(t, deletePath); got != "before delete\n" {
		t.Fatalf("expected deleted file restored, got %q", got)
	}

	oldPath := filepath.Join(workspace, "old.txt")
	newPath := filepath.Join(workspace, "nested", "new.txt")
	mustWriteRollbackTestFile(t, oldPath, "before move\n")
	moveOp, err := store.Begin(context.Background(), BeginOptions{Workspace: workspace, ToolName: "apply_patch", TraceID: "trace-move"}, []FileTarget{{
		Path:       "old.txt",
		AbsPath:    oldPath,
		NewPath:    "nested/new.txt",
		NewAbsPath: newPath,
		OpType:     OpTypeMove,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteRollbackTestFile(t, newPath, "after move\n")
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(context.Background(), moveOp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, moveOp.OperationID); err != nil {
		t.Fatal(err)
	}
	if got := mustReadRollbackTestFile(t, oldPath); got != "before move\n" {
		t.Fatalf("expected moved file restored, got %q", got)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("expected moved target removed, got %v", err)
	}
}

func mustWriteRollbackTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadRollbackTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
