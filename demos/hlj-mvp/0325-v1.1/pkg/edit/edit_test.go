package edit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareAndEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("old\nvalue\n"), 0644); err != nil {
		t.Fatal(err)
	}

	editor := New()
	preview, err := editor.Prepare(path, "new\nvalue\n")
	if err != nil {
		t.Fatal(err)
	}

	if preview.OldHash == "" {
		t.Fatal("expected existing file hash")
	}
	if !strings.Contains(preview.Diff, "-old") || !strings.Contains(preview.Diff, "+new") {
		t.Fatalf("unexpected diff: %s", preview.Diff)
	}

	result, err := editor.Edit(path, "new\nvalue\n", preview.OldHash)
	if err != nil {
		t.Fatal(err)
	}

	if result.NewHash == preview.OldHash {
		t.Fatal("expected hash to change after edit")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\nvalue\n" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}

func TestEditRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	editor := New()
	_, err := editor.Edit(path, "changed", "wrong-hash")
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
}
