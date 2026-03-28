package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
)

func TestStorePreservesUTF8Content(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	utf8Text := "\u4f60\u597d\uff0c\u4e16\u754c"
	sess := New(`E:\\workspace`)
	sess.Messages = append(sess.Messages, llm.Message{
		Role:    "user",
		Content: utf8Text,
	})

	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, sess.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), utf8Text) {
		t.Fatalf("expected raw json to contain utf-8 text %q, got %q", utf8Text, string(data))
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != utf8Text {
		t.Fatalf("expected utf-8 content after load, got %#v", loaded.Messages)
	}
}

func TestStoreListReturnsRecentSessions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	older := New(`E:\\repo-old`)
	older.ID = "older"
	older.CreatedAt = time.Date(2026, 3, 24, 8, 0, 0, 0, time.UTC)
	older.Messages = []llm.Message{{Role: "user", Content: "first question"}}
	if err := store.Save(older); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	newer := New(`E:\\repo-new`)
	newer.ID = "newer"
	newer.CreatedAt = time.Date(2026, 3, 24, 9, 0, 0, 0, time.UTC)
	newer.Messages = []llm.Message{{Role: "assistant", Content: "thinking"}, {Role: "user", Content: "second question with more detail"}}
	if err := store.Save(newer); err != nil {
		t.Fatal(err)
	}

	summaries, warnings, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].ID != "newer" {
		t.Fatalf("expected newest session first, got %#v", summaries)
	}
	if summaries[0].LastUserMessage != "second question with more detail" {
		t.Fatalf("unexpected preview: %#v", summaries[0])
	}
	if summaries[0].MessageCount != 2 {
		t.Fatalf("expected message count 2, got %#v", summaries[0])
	}

	limited, warnings, err := store.List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for limited list, got %#v", warnings)
	}
	if len(limited) != 1 || limited[0].ID != "newer" {
		t.Fatalf("expected limited list to keep newest summary, got %#v", limited)
	}
}

func TestStoreListSkipsEmptySessionFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(`E:\\repo`)
	sess.ID = "valid"
	sess.Messages = []llm.Message{{Role: "user", Content: "hello"}}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, warnings, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "valid" {
		t.Fatalf("expected valid session to remain visible, got %#v", summaries)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "empty.json") || !strings.Contains(warnings[0], "empty file") {
		t.Fatalf("unexpected warning: %q", warnings[0])
	}
}

func TestStoreListSkipsInvalidJSONSessionFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(`E:\\repo`)
	sess.ID = "valid"
	sess.Messages = []llm.Message{{Role: "user", Content: "hello"}}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, warnings, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "valid" {
		t.Fatalf("expected valid session to remain visible, got %#v", summaries)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "broken.json") || !strings.Contains(warnings[0], "invalid JSON") {
		t.Fatalf("unexpected warning: %q", warnings[0])
	}
}

func TestStoreSaveReplacesExistingSessionFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(`E:\\repo`)
	sess.ID = "stable"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	sess.Messages = append(sess.Messages, llm.Message{Role: "user", Content: "updated"})
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "updated" {
		t.Fatalf("expected updated session content, got %#v", loaded.Messages)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("expected no temp files left behind, found %s", entry.Name())
		}
	}
}

func TestSummarizeMessagePreservesUTF8WhenTruncating(t *testing.T) {
	text := "\u7ee7\u7eed\u521a\u624d\u7684\u4e0a\u4e0b\u6587\uff0c\u7ed9\u6211\u5217\u4e00\u4e0b\u5f53\u524d\u4e3b MVP \u6700\u5173\u952e\u7684\u6d4b\u8bd5\u70b9"
	got := summarizeMessage(text, 24)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid utf-8 preview, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", got)
	}
}
