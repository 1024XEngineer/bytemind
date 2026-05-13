package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyFileTypeAndImageExt(t *testing.T) {
	tests := []struct {
		path string
		want FileType
	}{
		{"photo.bmp", FileTypeImage},
		{"notes.md", FileTypeText},
		{"report.pdf", FileTypePDF},
		{"archive.bin", FileTypeUnknown},
	}

	for _, tc := range tests {
		if got := classifyFileType(tc.path); got != tc.want {
			t.Fatalf("classifyFileType(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
	if !hasImageExt("photo.bmp") {
		t.Fatal("expected bmp to be treated as an image")
	}
	if hasImageExt("notes.md") {
		t.Fatal("did not expect text file to be treated as an image")
	}
}

func TestImageInputControllerProcessMutationReplacesInlineImagePath(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "inline.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	controller := NewImageInputController()
	updated, note := controller.ProcessMutation("ask ", "ask "+imagePath+" please", "paste", dir, func(path string) (string, string, bool) {
		if path != filepath.Clean(imagePath) {
			t.Fatalf("unexpected ingest path %q", path)
		}
		return "[Image#1]", "", true
	})

	if updated != "ask [Image#1] please" {
		t.Fatalf("expected inline path replacement, got %q", updated)
	}
	if !strings.Contains(note, "Attached 1 image") {
		t.Fatalf("expected attach note, got %q", note)
	}
}

func TestImageInputControllerProcessMutationReturnsFailureNote(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	controller := NewImageInputController()
	updated, note := controller.ProcessMutation("", imagePath, "paste", dir, func(string) (string, string, bool) {
		return "", "ingest failed", false
	})

	if updated != imagePath {
		t.Fatalf("expected failed path to remain unchanged, got %q", updated)
	}
	if note != "ingest failed" {
		t.Fatalf("expected failure note, got %q", note)
	}
}

func TestImageInputControllerProcessWholeInputFallback(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "fallback.gif")
	if err := os.WriteFile(imagePath, []byte("gif"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	controller := NewImageInputController()
	unchanged, note := controller.ProcessWholeInputFallback(imagePath, "rune", time.Time{}, 400*time.Millisecond, func(string) (string, string, bool) {
		t.Fatal("ingest should not be called without paste signal")
		return "", "", false
	})
	if unchanged != imagePath || note != "" {
		t.Fatalf("expected no fallback without paste signal, got updated=%q note=%q", unchanged, note)
	}

	updated, note := controller.ProcessWholeInputFallback("see "+imagePath, "paste", time.Time{}, 400*time.Millisecond, func(path string) (string, string, bool) {
		if path != filepath.Clean(imagePath) {
			t.Fatalf("unexpected ingest path %q", path)
		}
		return "[Image#1]", "", true
	})
	if updated != "see [Image#1]" {
		t.Fatalf("expected fallback replacement, got %q", updated)
	}
	if !strings.Contains(note, "Attached 1 image") {
		t.Fatalf("expected attach note, got %q", note)
	}
}

func TestImageInputControllerAttachClipboard(t *testing.T) {
	controller := NewImageInputController()
	updated, note := controller.AttachClipboard("prefix", "image/png", "clip.png", []byte("png"), func(mediaType, fileName string, data []byte) (string, string, bool) {
		if mediaType != "image/png" || fileName != "clip.png" || string(data) != "png" {
			t.Fatalf("unexpected clipboard payload: %q %q %q", mediaType, fileName, string(data))
		}
		return "[Image#1]", "", true
	})

	if updated != "prefix [Image#1]" {
		t.Fatalf("expected placeholder appended, got %q", updated)
	}
	if !strings.Contains(note, "Attached image from clipboard") {
		t.Fatalf("expected clipboard note, got %q", note)
	}
}
