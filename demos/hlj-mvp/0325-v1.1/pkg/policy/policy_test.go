package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckWriteRejectsSensitiveFiles(t *testing.T) {
	p := New(t.TempDir())

	if err := p.CheckWrite(".env"); err == nil {
		t.Fatal("expected sensitive file write to be rejected")
	}
	if err := p.CheckWrite("src/app.go"); err != nil {
		t.Fatalf("expected normal file to be allowed, got %v", err)
	}
}

func TestCheckExecRejectsBlockedCommands(t *testing.T) {
	p := New(t.TempDir())

	if err := p.CheckExec("git reset --hard HEAD"); err == nil {
		t.Fatal("expected blocked command to be rejected")
	}
	if err := p.CheckExec("git config --global --add safe.directory D:/"); err == nil {
		t.Fatal("expected global git config command to be rejected")
	}
	if err := p.CheckExec("go test ./..."); err != nil {
		t.Fatalf("expected normal command to be allowed, got %v", err)
	}
}

func TestLogIncludesSessionID(t *testing.T) {
	root := t.TempDir()
	p := New(root)

	if err := p.Log("session-123", "write_file", "main.go", true, true, "ok"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".forgecli", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"session_id":"session-123"`) {
		t.Fatalf("expected session id in audit log, got %s", string(data))
	}
}
