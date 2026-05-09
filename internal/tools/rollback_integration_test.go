package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
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
