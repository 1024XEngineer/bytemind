package storage

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAndReadJSONLSnapshot(t *testing.T) {
	store, err := NewSessionFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "sessions", "s1.jsonl")
	at := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"id":        "s1",
		"workspace": "C:/repo",
	}
	if err := WriteJSONLSnapshot(store, path, "session_snapshot", 1, payload, at); err != nil {
		t.Fatalf("WriteJSONLSnapshot failed: %v", err)
	}
	raw, err := ReadLatestJSONLSnapshot(store, path, "session_snapshot")
	if err != nil {
		t.Fatalf("ReadLatestJSONLSnapshot failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal payload failed: %v", err)
	}
	if got["id"] != "s1" {
		t.Fatalf("unexpected payload id: %#v", got)
	}
}

func TestReadLatestJSONLSnapshotSkipsInvalidOrWrongType(t *testing.T) {
	store, err := NewSessionFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "sessions", "mixed.jsonl")
	content := strings.Join([]string{
		"{bad}",
		`{"v":1,"ts":"2026-04-13T11:00:00Z","type":"other","payload":{"id":"old"}}`,
		`{"v":1,"ts":"2026-04-13T11:00:01Z","type":"session_snapshot","payload":{"id":"new"}}`,
		"",
	}, "\n")
	if err := store.WriteAtomic(path, []byte(content)); err != nil {
		t.Fatalf("WriteAtomic fixture failed: %v", err)
	}
	raw, err := ReadLatestJSONLSnapshot(store, path, "session_snapshot")
	if err != nil {
		t.Fatalf("ReadLatestJSONLSnapshot failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal payload failed: %v", err)
	}
	if got["id"] != "new" {
		t.Fatalf("expected newest matching payload, got %#v", got)
	}
}
