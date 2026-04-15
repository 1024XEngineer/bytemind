package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
	storagepkg "bytemind/internal/storage"
)

func TestStorePreservesUTF8Content(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	utf8Text := "你好，世界"
	sess := New(`E:\\workspace`)
	sess.Messages = append(sess.Messages, llm.Message{Role: "user", Content: utf8Text})

	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, storagepkg.WorkspaceProjectID(sess.Workspace), sess.ID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), utf8Text) {
		t.Fatalf("expected raw jsonl to contain utf-8 text %q, got %q", utf8Text, string(data))
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

	brokenDir := filepath.Join(dir, storagepkg.WorkspaceProjectID(sess.Workspace))
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "empty.jsonl"), nil, 0o644); err != nil {
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
	if !strings.Contains(warnings[0], "empty.jsonl") {
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

	brokenDir := filepath.Join(dir, storagepkg.WorkspaceProjectID(sess.Workspace))
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "broken.jsonl"), []byte("{"), 0o644); err != nil {
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
	if !strings.Contains(warnings[0], "broken.jsonl") {
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

	projectDir := filepath.Join(dir, storagepkg.WorkspaceProjectID(sess.Workspace))
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("expected no temp files left behind, found %s", entry.Name())
		}
	}
}

func TestStorePersistsActiveSkill(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(`E:\\repo`)
	sess.ActiveSkill = &ActiveSkill{
		Name: "review",
		Args: map[string]string{
			"base_ref": "main",
		},
		ActivatedAt: time.Date(2026, 4, 3, 10, 20, 0, 0, time.UTC),
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveSkill == nil {
		t.Fatal("expected active skill to be persisted")
	}
	if loaded.ActiveSkill.Name != "review" {
		t.Fatalf("unexpected active skill name: %#v", loaded.ActiveSkill)
	}
	if loaded.ActiveSkill.Args["base_ref"] != "main" {
		t.Fatalf("unexpected active skill args: %#v", loaded.ActiveSkill.Args)
	}
}

func TestStoreListUserPreviewIgnoresToolResultPayload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(`E:\\repo`)
	sess.ID = "preview"
	sess.Messages = []llm.Message{
		llm.NewUserTextMessage("real user text"),
		llm.NewToolResultMessage("call-1", `{"ok":true}`),
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	summaries, _, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %#v", summaries)
	}
	if summaries[0].LastUserMessage != "real user text" {
		t.Fatalf("expected preview from user text part, got %#v", summaries[0])
	}
}

func TestStoreSaveRejectsInvalidTimelineMessage(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess := New(`E:\\repo`)
	sess.Messages = []llm.Message{{
		Role: llm.RoleAssistant,
		Parts: []llm.Part{{
			Type:  llm.PartImageRef,
			Image: &llm.ImagePartRef{AssetID: "asset-1"},
		}},
	}}
	if err := store.Save(sess); err == nil {
		t.Fatal("expected validation failure for invalid assistant image_ref")
	}
}

func TestStoreIgnoresLegacyJSONFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(legacy, []byte(`{"id":"legacy","workspace":"E:\\repo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, warnings, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected no summaries from legacy json, got %#v", summaries)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for ignored legacy json files, got %#v", warnings)
	}
	if _, err := store.Load("legacy"); !os.IsNotExist(err) {
		t.Fatalf("expected legacy json to be unsupported and not found, got %v", err)
	}
}

func TestSummarizeMessagePreservesUTF8WhenTruncating(t *testing.T) {
	text := "继续刚才的上下文，给我列一下当前主 MVP 最关键的测试点"
	got := summarizeMessage(text, 24)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid utf-8 preview, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", got)
	}
}
