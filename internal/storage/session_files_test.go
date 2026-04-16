package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionFileStoreWriteAtomicAndRead(t *testing.T) {
	store, err := NewSessionFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "a", "b.jsonl")
	if err := store.WriteAtomic(path, []byte("hello\n")); err != nil {
		t.Fatalf("WriteAtomic failed: %v", err)
	}
	data, err := store.Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestSessionFileStoreAppendLine(t *testing.T) {
	root := t.TempDir()
	store, err := NewSessionFileStore(root)
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	path := filepath.Join(root, "events", "events.jsonl")
	if err := store.AppendLine(path, []byte(`{"seq":1}`)); err != nil {
		t.Fatalf("AppendLine first failed: %v", err)
	}
	if err := store.AppendLine(path, []byte(`{"seq":2}`)); err != nil {
		t.Fatalf("AppendLine second failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	got := string(data)
	want := "{\"seq\":1}\n{\"seq\":2}\n"
	if got != want {
		t.Fatalf("unexpected content\nwant: %q\n got: %q", want, got)
	}
}

func TestSessionFileStoreFindNewestByName(t *testing.T) {
	root := t.TempDir()
	store, err := NewSessionFileStore(root)
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	older := filepath.Join(root, "p1", "id.jsonl")
	newer := filepath.Join(root, "p2", "id.jsonl")
	if err := store.WriteAtomic(older, []byte("old\n")); err != nil {
		t.Fatalf("write older failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := store.WriteAtomic(newer, []byte("new\n")); err != nil {
		t.Fatalf("write newer failed: %v", err)
	}
	path, err := store.FindNewestByName("id.jsonl")
	if err != nil {
		t.Fatalf("FindNewestByName failed: %v", err)
	}
	if filepath.Clean(path) != filepath.Clean(newer) {
		t.Fatalf("expected newer path %q, got %q", newer, path)
	}
}

func TestSessionFileStoreListByExt(t *testing.T) {
	root := t.TempDir()
	store, err := NewSessionFileStore(root)
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.jsonl"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write a.jsonl failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write b.txt failed: %v", err)
	}
	paths, err := store.ListByExt(".jsonl")
	if err != nil {
		t.Fatalf("ListByExt failed: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 jsonl file, got %d", len(paths))
	}
}

func TestSessionFileStoreSessionPath(t *testing.T) {
	root := t.TempDir()
	store, err := NewSessionFileStore(root)
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	path, err := store.SessionPath("-project", "sess-1")
	if err != nil {
		t.Fatalf("SessionPath failed: %v", err)
	}
	expected := filepath.Join(root, "-project", "sess-1.jsonl")
	if filepath.Clean(path) != filepath.Clean(expected) {
		t.Fatalf("expected %q, got %q", expected, path)
	}
	if _, err := store.SessionPath("", "sess-1"); err == nil {
		t.Fatal("expected empty project id to fail")
	}
	if _, err := store.SessionPath("-project", ""); err == nil {
		t.Fatal("expected empty session id to fail")
	}
}

func TestSessionFileStoreRoot(t *testing.T) {
	root := t.TempDir()
	store, err := NewSessionFileStore(root)
	if err != nil {
		t.Fatalf("NewSessionFileStore failed: %v", err)
	}
	if got := store.Root(); filepath.Clean(got) != filepath.Clean(root) {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}
