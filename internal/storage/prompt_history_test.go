package storage

import (
	"os"
	"testing"
	"time"
)

func TestPromptHistoryStoreAppendAndLoadRecent(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	store, err := NewDefaultPromptHistoryStore()
	if err != nil {
		t.Fatalf("NewDefaultPromptHistoryStore failed: %v", err)
	}
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	if err := store.Append("/repo", "sess-1", "first", now); err != nil {
		t.Fatalf("Append first failed: %v", err)
	}
	if err := store.Append("/repo", "sess-2", "second", now.Add(time.Second)); err != nil {
		t.Fatalf("Append second failed: %v", err)
	}
	entries, err := store.LoadRecent(10)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Prompt != "first" || entries[1].Prompt != "second" {
		t.Fatalf("unexpected prompts: %#v", entries)
	}
}

func TestPromptHistoryHelpersAppendAndLoadRecent(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	if err := AppendPrompt("/repo", "sess-1", "first prompt", now); err != nil {
		t.Fatalf("AppendPrompt first failed: %v", err)
	}
	if err := AppendPrompt("/repo", "sess-2", "second prompt", now.Add(time.Second)); err != nil {
		t.Fatalf("AppendPrompt second failed: %v", err)
	}
	entries, err := LoadRecentPrompts(10)
	if err != nil {
		t.Fatalf("LoadRecentPrompts failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Prompt != "first prompt" || entries[1].Prompt != "second prompt" {
		t.Fatalf("unexpected prompts: %#v", entries)
	}
}

func TestPromptHistoryStoreSkipsCorruptedLines(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	path, err := DefaultPromptHistoryPath()
	if err != nil {
		t.Fatalf("DefaultPromptHistoryPath failed: %v", err)
	}
	content := []byte("{bad json}\n" +
		"{\"prompt\":\"\",\"session_id\":\"s\"}\n" +
		"{\"prompt\":\"ok\",\"session_id\":\"s\",\"timestamp\":\"2026-04-05T10:00:00Z\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	store, err := NewDefaultPromptHistoryStore()
	if err != nil {
		t.Fatalf("NewDefaultPromptHistoryStore failed: %v", err)
	}
	entries, err := store.LoadRecent(10)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Prompt != "ok" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}
