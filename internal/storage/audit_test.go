package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestFileAuditStoreAppendWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileAuditStore(dir)
	if err != nil {
		t.Fatalf("NewFileAuditStore failed: %v", err)
	}
	event := AuditEvent{
		SessionID: corepkg.SessionID("sess-1"),
		Actor:     "agent",
		Action:    "tool_execute_start",
	}
	if err := store.Append(context.Background(), event); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one audit day file, got %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatal("expected non-empty audit json line")
	}

	var decoded AuditEvent
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("invalid json payload: %v", err)
	}
	if decoded.EventID == "" {
		t.Fatal("expected auto-generated event_id")
	}
	if decoded.Action != event.Action {
		t.Fatalf("unexpected action: got=%q want=%q", decoded.Action, event.Action)
	}
}
