package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
	"github.com/1024XEngineer/bytemind/internal/session"
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

func TestRunRollbackCommandRecordsExchange(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	m := model{
		workspace: workspace,
		store:     store,
		sess:      sess,
	}

	if err := m.runRollbackCommand("/rollback"); err != nil {
		t.Fatal(err)
	}
	if m.statusNote != "Rollback operations listed." {
		t.Fatalf("expected rollback list status, got %q", m.statusNote)
	}
	if len(m.chatItems) != 2 || !strings.Contains(m.chatItems[1].Body, "No ByteMind rollback operations") {
		t.Fatalf("expected rollback exchange, got %#v", m.chatItems)
	}
	if len(sess.Messages) != 2 || sess.Messages[0].Text() != "/rollback" {
		t.Fatalf("expected rollback exchange recorded in session, got %#v", sess.Messages)
	}
}

func TestRunRollbackCommandHandlesErrorsAndSaveFailure(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	m := model{workspace: workspace, sess: session.New(workspace), store: failingCommitSessionStore{}}

	if err := m.runRollbackCommand("/rollback bad id"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.chatItems[1].Body, "Usage: /rollback") || !strings.Contains(m.statusNote, "session save failed") {
		t.Fatalf("expected error exchange and save failure status, got body=%q status=%q", m.chatItems[1].Body, m.statusNote)
	}
}

func TestHandleSlashCommandRollback(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := model{workspace: workspace, store: store, sess: session.New(workspace)}
	if err := m.handleSlashCommand("/rollback"); err != nil {
		t.Fatal(err)
	}
	if m.statusNote != "Rollback operations listed." {
		t.Fatalf("expected rollback command to run, got %q", m.statusNote)
	}
}

func TestExecuteRollbackCommandAdditionalBranches(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	workspace := t.TempDir()
	if _, _, err := executeRollbackCommand(context.Background(), workspace, nil, "/not-rollback"); err == nil || err.Error() != rollbackUsage {
		t.Fatalf("expected rollback usage error, got %v", err)
	}
	if _, _, err := executeRollbackCommand(context.Background(), workspace, nil, "/rollback missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing rollback operation error, got %v", err)
	}

	blockFile := filepath.Join(t.TempDir(), "home-as-file")
	if err := os.WriteFile(blockFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYTEMIND_HOME", blockFile)
	if _, _, err := executeRollbackCommand(context.Background(), workspace, nil, "/rollback"); err == nil || !strings.Contains(err.Error(), "Rollback unavailable") {
		t.Fatalf("expected unavailable rollback store, got %v", err)
	}
}

func TestRollbackFormattingBranches(t *testing.T) {
	if got := formatRollbackList(nil); !strings.Contains(got, "No ByteMind rollback operations") {
		t.Fatalf("expected empty rollback list message, got %q", got)
	}
	if got := shortRollbackID("short-id"); got != "short-id" {
		t.Fatalf("expected short id unchanged, got %q", got)
	}

	op := rollbackpkg.Operation{
		OperationID: "20260510T000000.000000000Z-abcdef",
		ToolName:    "apply_patch",
		AffectedFiles: []rollbackpkg.FileChange{
			{Path: "a.txt"},
			{Path: "old.txt", NewPath: "new.txt", OpType: rollbackpkg.OpTypeMove},
			{Path: "c.txt"},
			{Path: "d.txt"},
		},
	}
	summary := rollbackPathSummary(op)
	if !strings.Contains(summary, "old.txt -> new.txt") || !strings.Contains(summary, "+1 more") {
		t.Fatalf("expected move and truncation summary, got %q", summary)
	}
	success := formatRollbackSuccess(op)
	if !strings.Contains(success, "Rollback completed.") || !strings.Contains(success, "Files restored: 4") {
		t.Fatalf("expected rollback success details, got %q", success)
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
