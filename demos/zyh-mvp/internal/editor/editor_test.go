package editor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRejectsConflicts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "demo.txt")
	initial := []byte("hello\n")
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	expectedHash := hashBytes(initial)
	if err := os.WriteFile(path, []byte("external change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := Service{}
	err := service.Apply(path, expectedHash, []byte("new content\n"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
