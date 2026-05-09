package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
)

func TestExecuteRollbackCommandListsAndRollsBackLast(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	path := filepath.Join(workspace, "file.txt")
	writeRollbackCommandTestFile(t, path, "old\n")

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	op, err := store.Begin(context.Background(), rollbackpkg.BeginOptions{
		Workspace: workspace,
		ToolName:  "write_file",
		TraceID:   "trace-rollback-command",
	}, []rollbackpkg.FileTarget{{
		Path:    "file.txt",
		AbsPath: path,
		OpType:  rollbackpkg.OpTypeUpdate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	writeRollbackCommandTestFile(t, path, "new\n")
	if err := store.Commit(context.Background(), op); err != nil {
		t.Fatal(err)
	}

	response, status, err := executeRollbackCommand(context.Background(), workspace, nil, "/rollback")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response, shortRollbackID(op.OperationID)) || status != "Rollback operations listed." {
		t.Fatalf("expected rollback list response, got %q / %q", response, status)
	}

	response, status, err = executeRollbackCommand(context.Background(), workspace, nil, "/rollback last")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response, "Rollback completed.") || status != "Rollback completed." {
		t.Fatalf("expected rollback success response, got %q / %q", response, status)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("expected file restored, got %q", string(data))
	}
}

func TestExecuteRollbackCommandRejectsInvalidUsage(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if _, _, err := executeRollbackCommand(context.Background(), t.TempDir(), nil, "/rollback a b"); err == nil || err.Error() != rollbackUsage {
		t.Fatalf("expected rollback usage error, got %v", err)
	}
}

func writeRollbackCommandTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
