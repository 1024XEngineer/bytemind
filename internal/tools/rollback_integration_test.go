package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
	"github.com/1024XEngineer/bytemind/internal/session"
)

func TestWriteFileToolRecordsRollbackOperation(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	tool := WriteFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":    "new.txt",
		"content": "created\n",
	})

	result, err := tool.Run(context.Background(), payload, &ExecutionContext{
		Workspace: workspace,
		RunID:     "trace-write",
	})
	if err != nil {
		t.Fatal(err)
	}
	operationID := rollbackOperationIDFromToolResult(t, result)
	if operationID == "" {
		t.Fatal("expected rollback operation id")
	}

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected rollback to remove created file, got %v", err)
	}
}

func TestReplaceInFileToolRecordsRollbackOperation(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	mustWriteFile(t, path, "alpha beta\n")
	tool := ReplaceInFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path": "sample.txt",
		"old":  "beta",
		"new":  "gamma",
	})

	result, err := tool.Run(context.Background(), payload, &ExecutionContext{
		Workspace: workspace,
		RunID:     "trace-replace",
	})
	if err != nil {
		t.Fatal(err)
	}
	operationID := rollbackOperationIDFromToolResult(t, result)

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, operationID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha beta\n" {
		t.Fatalf("expected replacement rollback, got %q", string(data))
	}
}

func TestApplyPatchToolRollbackRestoresMultipleFiles(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "a.txt"), "alpha\nbeta\n")
	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.txt",
			"@@",
			" alpha",
			"-beta",
			"+gamma",
			"*** Add File: b.txt",
			"+created",
			"*** End Patch",
		}, "\n"),
	})

	result, err := tool.Run(context.Background(), payload, &ExecutionContext{
		Workspace: workspace,
		RunID:     "trace-patch",
	})
	if err != nil {
		t.Fatal(err)
	}
	operationID := rollbackOperationIDFromToolResult(t, result)

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rollback(context.Background(), workspace, operationID); err != nil {
		t.Fatal(err)
	}
	if got := mustReadToolsTestFile(t, filepath.Join(workspace, "a.txt")); got != "alpha\nbeta\n" {
		t.Fatalf("expected a.txt restored, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected b.txt removed, got %v", err)
	}
}

func TestApplyPatchToolFailureAutomaticallyRestoresChangedFiles(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	mustWriteFile(t, path, "alpha\nbeta\n")
	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.txt",
			"@@",
			" alpha",
			"-beta",
			"+gamma",
			"*** Delete File: missing.txt",
			"*** End Patch",
		}, "\n"),
	})

	_, err := tool.Run(context.Background(), payload, &ExecutionContext{
		Workspace: workspace,
		RunID:     "trace-patch-failure",
	})
	if err == nil {
		t.Fatal("expected patch failure")
	}
	if got := mustReadToolsTestFile(t, path); got != "alpha\nbeta\n" {
		t.Fatalf("expected failed patch to restore original content, got %q", got)
	}

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := store.ListRecent(context.Background(), workspace, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("expected failed patch not to leave committed rollback operations, got %#v", ops)
	}
}

func TestRollbackHelperBranches(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	mustWriteFile(t, path, "before\n")

	if rollbackEnabled(nil) {
		t.Fatal("expected nil execution context to disable rollback")
	}
	if tracker, err := beginRollbackOperation(context.Background(), &ExecutionContext{Workspace: workspace}, "write_file", []rollbackpkg.FileTarget{{
		Path:    "file.txt",
		AbsPath: path,
		OpType:  rollbackpkg.OpTypeUpdate,
	}}); err != nil || tracker != nil {
		t.Fatalf("expected rollback disabled without run id, got %#v / %v", tracker, err)
	}
	if tracker, err := beginRollbackOperation(context.Background(), &ExecutionContext{Workspace: workspace, RunID: "trace-empty"}, "write_file", nil); err != nil || tracker != nil {
		t.Fatalf("expected empty targets to skip rollback, got %#v / %v", tracker, err)
	}
	if got := rollbackSessionID(&ExecutionContext{Session: session.New(workspace)}); got == "" {
		t.Fatal("expected session id from execution context")
	}
	if id, err := (*rollbackTracker)(nil).commit(context.Background()); err != nil || id != "" {
		t.Fatalf("expected nil tracker commit no-op, got %q / %v", id, err)
	}
	(*rollbackTracker)(nil).abort(context.Background(), "ignored")
}

func TestRollbackHelperPropagatesBeginAndCommitFailures(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	binaryPath := filepath.Join(workspace, "binary.bin")
	mustWriteFile(t, binaryPath, "a\x00b")

	if _, err := beginRollbackOperation(context.Background(), &ExecutionContext{Workspace: workspace, RunID: "trace-binary"}, "write_file", []rollbackpkg.FileTarget{{
		Path:    "binary.bin",
		AbsPath: binaryPath,
		OpType:  rollbackpkg.OpTypeUpdate,
	}}); err == nil || !strings.Contains(err.Error(), "not a text") {
		t.Fatalf("expected begin rollback snapshot error, got %v", err)
	}

	textPath := filepath.Join(workspace, "text.txt")
	mustWriteFile(t, textPath, "before\n")
	tracker, err := beginRollbackOperation(context.Background(), &ExecutionContext{Workspace: workspace, RunID: "trace-missing"}, "write_file", []rollbackpkg.FileTarget{{
		Path:    "text.txt",
		AbsPath: textPath,
		OpType:  rollbackpkg.OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(textPath); err != nil {
		t.Fatal(err)
	}
	_, err = tracker.commit(context.Background())
	if err == nil || !strings.Contains(err.Error(), "absent after update") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if got := mustReadToolsTestFile(t, textPath); got != "before\n" {
		t.Fatalf("expected failed commit to restore snapshot, got %q", got)
	}
}

func rollbackOperationIDFromToolResult(t *testing.T, result string) string {
	t.Helper()
	var parsed struct {
		OperationID string `json:"rollback_operation_id"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(parsed.OperationID)
}

func mustReadToolsTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
