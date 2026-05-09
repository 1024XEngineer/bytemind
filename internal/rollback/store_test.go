package rollback

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestDefaultStoreListsRecentAndRollsBackLast(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}

	firstPath := filepath.Join(workspace, "first.txt")
	secondPath := filepath.Join(workspace, "second.txt")
	ignoredPath := filepath.Join(workspace, "ignored.txt")
	mustWriteRollbackTestFile(t, firstPath, "first old\n")
	mustWriteRollbackTestFile(t, secondPath, "second old\n")
	mustWriteRollbackTestFile(t, ignoredPath, "ignored old\n")

	first := beginUpdateRollbackTestOperation(t, store, workspace, "first.txt", firstPath)
	mustWriteRollbackTestFile(t, firstPath, "first new\n")
	if err := store.Commit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := beginUpdateRollbackTestOperation(t, store, workspace, "second.txt", secondPath)
	mustWriteRollbackTestFile(t, secondPath, "second new\n")
	if err := store.Commit(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	ignored := beginUpdateRollbackTestOperation(t, store, workspace, "ignored.txt", ignoredPath)
	if err := store.Abort(context.Background(), ignored, "not committed"); err != nil {
		t.Fatal(err)
	}

	ops, err := store.ListRecent(context.Background(), workspace, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].OperationID != second.OperationID {
		t.Fatalf("expected only newest committed operation, got %#v", ops)
	}

	rolledBack, err := store.RollbackLast(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.OperationID != second.OperationID {
		t.Fatalf("expected latest operation %s, got %s", second.OperationID, rolledBack.OperationID)
	}
	if got := mustReadRollbackTestFile(t, secondPath); got != "second old\n" {
		t.Fatalf("expected second file restored, got %q", got)
	}
	if got := mustReadRollbackTestFile(t, firstPath); got != "first new\n" {
		t.Fatalf("expected first file untouched, got %q", got)
	}
}

func TestAbortAndRestoreRestoresPendingOperation(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "file.txt")
	mustWriteRollbackTestFile(t, path, "before\n")

	op := beginUpdateRollbackTestOperation(t, store, workspace, "file.txt", path)
	mustWriteRollbackTestFile(t, path, "partial write\n")
	if err := store.AbortAndRestore(context.Background(), op, "write failed"); err != nil {
		t.Fatal(err)
	}
	if op.Status != StatusAborted {
		t.Fatalf("expected aborted status, got %s", op.Status)
	}
	if got := mustReadRollbackTestFile(t, path); got != "before\n" {
		t.Fatalf("expected pending change restored, got %q", got)
	}
}

func TestAbortAndRestoreMarksFailureWhenPathEscapesRoots(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.txt")
	mustWriteRollbackTestFile(t, external, "before\n")
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	op, err := store.Begin(context.Background(), BeginOptions{
		Workspace: workspace,
		ToolName:  "write_file",
		TraceID:   "trace-external",
	}, []FileTarget{{
		Path:    filepath.ToSlash(external),
		AbsPath: external,
		OpType:  OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteRollbackTestFile(t, external, "partial\n")

	err = store.AbortAndRestore(context.Background(), op, "write failed")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected path validation failure, got %v", err)
	}
	if op.Status != StatusRollbackFailed {
		t.Fatalf("expected rollback_failed status, got %s", op.Status)
	}
	if got := mustReadRollbackTestFile(t, external); got != "partial\n" {
		t.Fatalf("expected failed restore to leave external file, got %q", got)
	}
}

func TestListRecentHandlesCanceledContextAndNoOperations(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	if ops, err := store.ListRecent(context.Background(), workspace, 0); err != nil || len(ops) != 0 {
		t.Fatalf("expected empty list, got %#v / %v", ops, err)
	}
	if _, err := store.RollbackLast(context.Background(), workspace); err == nil || !strings.Contains(err.Error(), "no committed") {
		t.Fatalf("expected no committed operation error, got %v", err)
	}

	path := filepath.Join(workspace, "file.txt")
	mustWriteRollbackTestFile(t, path, "before\n")
	op := beginUpdateRollbackTestOperation(t, store, workspace, "file.txt", path)
	mustWriteRollbackTestFile(t, path, "after\n")
	if err := store.Commit(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ListRecent(ctx, workspace, 10); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestStoreRejectsInvalidInputsAndSnapshotTargets(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if _, err := NewStore(" "); err == nil {
		t.Fatal("expected empty rollback root error")
	}
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(context.Background(), BeginOptions{Workspace: t.TempDir()}, nil); err == nil {
		t.Fatal("expected empty target error")
	}
	if _, err := (*Store)(nil).Begin(context.Background(), BeginOptions{}, []FileTarget{{Path: "x"}}); err == nil {
		t.Fatal("expected nil store error")
	}
	if err := store.saveOperation(&Operation{}); err == nil {
		t.Fatal("expected missing operation id error")
	}

	workspace := t.TempDir()
	dirTarget := filepath.Join(workspace, "dir")
	if err := os.MkdirAll(dirTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(context.Background(), BeginOptions{Workspace: workspace}, []FileTarget{{Path: "dir", AbsPath: dirTarget}}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory snapshot error, got %v", err)
	}
	binaryTarget := filepath.Join(workspace, "binary.bin")
	if err := os.WriteFile(binaryTarget, []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(context.Background(), BeginOptions{Workspace: workspace}, []FileTarget{{Path: "binary.bin", AbsPath: binaryTarget}}); err == nil || !strings.Contains(err.Error(), "not a text") {
		t.Fatalf("expected binary snapshot error, got %v", err)
	}
}

func TestFindOperationAndLoadErrors(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.findOperation(workspace, " "); err == nil {
		t.Fatal("expected empty operation id error")
	}
	if _, err := store.findOperation(workspace, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}

	first := beginUpdateRollbackTestOperation(t, store, workspace, "a.txt", filepath.Join(workspace, "a.txt"))
	second := beginUpdateRollbackTestOperation(t, store, workspace, "b.txt", filepath.Join(workspace, "b.txt"))
	first.OperationID = "same-prefix-one"
	second.OperationID = "same-prefix-two"
	if err := store.saveOperation(first); err != nil {
		t.Fatal(err)
	}
	if err := store.saveOperation(second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.findOperation(workspace, "same-prefix"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous prefix error, got %v", err)
	}

	if err := os.WriteFile(filepath.Join(store.entriesDir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadAll(); err == nil || !strings.Contains(err.Error(), "read rollback operation") {
		t.Fatalf("expected invalid json load error, got %v", err)
	}
}

func TestRollbackErrorsAndHelperBranches(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollback"))
	if err != nil {
		t.Fatal(err)
	}

	unsupported := &Operation{
		OperationID:   "unsupported",
		Workspace:     workspace,
		Status:        StatusPending,
		CreatedAt:     timeNowForRollbackTest(),
		UpdatedAt:     timeNowForRollbackTest(),
		AffectedFiles: []FileChange{{Path: "x.txt", OpType: OpType("bad")}},
	}
	if err := store.Commit(context.Background(), unsupported); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported commit error, got %v", err)
	}

	committed := beginUpdateRollbackTestOperation(t, store, workspace, "file.txt", filepath.Join(workspace, "file.txt"))
	committed.Status = StatusRolledBack
	if err := store.saveOperation(committed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, committed.OperationID); err == nil || !strings.Contains(err.Error(), "not committed") {
		t.Fatalf("expected non-committed rollback error, got %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	mustWriteRollbackTestFile(t, outside, "outside\n")
	external := beginUpdateRollbackTestOperation(t, store, workspace, filepath.ToSlash(outside), outside)
	external.Status = StatusCommitted
	external.AffectedFiles[0].AfterHash = hashBytes([]byte("outside\n"))
	if err := store.saveOperation(external); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, external.OperationID); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected path escape rollback error, got %v", err)
	}

	path := filepath.Join(workspace, "state.txt")
	states := map[string]snapshotState{
		path:                                    {exists: true, data: []byte("restored\n")},
		filepath.Join(workspace, "missing.txt"): {},
	}
	if err := restoreCurrentStates(states); err != nil {
		t.Fatal(err)
	}
	if got := mustReadRollbackTestFile(t, path); got != "restored\n" {
		t.Fatalf("expected restored current state, got %q", got)
	}

	insideDisplay := displayPath(workspace, filepath.Join(workspace, "nested", "file.txt"))
	if insideDisplay != "nested/file.txt" {
		t.Fatalf("expected relative display path, got %q", insideDisplay)
	}
	outsideDisplay := displayPath(workspace, outside)
	if outsideDisplay != filepath.ToSlash(outside) {
		t.Fatalf("expected absolute display path, got %q", outsideDisplay)
	}
}

func beginUpdateRollbackTestOperation(t *testing.T, store *Store, workspace, relPath, absPath string) *Operation {
	t.Helper()
	op, err := store.Begin(context.Background(), BeginOptions{
		Workspace: workspace,
		ToolName:  "write_file",
		TraceID:   "trace-test",
	}, []FileTarget{{
		Path:    relPath,
		AbsPath: absPath,
		OpType:  OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func timeNowForRollbackTest() time.Time {
	return time.Now().UTC()
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
