package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineCount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.txt")
	os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0644)
	if got := lineCount(p); got != 3 {
		t.Errorf("lineCount = %d, want 3", got)
	}

	empty := filepath.Join(dir, "empty.txt")
	os.WriteFile(empty, []byte(""), 0644)
	if got := lineCount(empty); got != 0 {
		t.Errorf("lineCount empty = %d, want 0", got)
	}

	if got := lineCount("/nonexistent/path"); got != 0 {
		t.Errorf("lineCount nonexistent = %d, want 0", got)
	}
}

func TestTruncateDiff(t *testing.T) {
	// Under limits — no truncation
	files := []DiffFile{{
		Hunks: []DiffHunk{{Lines: []string{"+a", "+b"}}},
	}}
	if truncateDiff(files) {
		t.Error("small diff should not truncate")
	}

	// Over max hunks per file
	manyHunks := make([]DiffHunk, diffMaxHunksPerFile+5)
	for i := range manyHunks {
		manyHunks[i] = DiffHunk{Lines: []string{"+x"}}
	}
	files2 := []DiffFile{{Hunks: manyHunks}}
	if !truncateDiff(files2) {
		t.Error("should truncate when over max hunks per file")
	}
	if len(files2[0].Hunks) != diffMaxHunksPerFile {
		t.Errorf("got %d hunks, want %d", len(files2[0].Hunks), diffMaxHunksPerFile)
	}

	// Over max lines per hunk
	longHunk := DiffHunk{Lines: make([]string, diffMaxLinesPerHunk+10)}
	files3 := []DiffFile{{Hunks: []DiffHunk{longHunk}}}
	if !truncateDiff(files3) {
		t.Error("should truncate when over max lines per hunk")
	}
	if len(files3[0].Hunks[0].Lines) != diffMaxLinesPerHunk {
		t.Errorf("got %d lines, want %d", len(files3[0].Hunks[0].Lines), diffMaxLinesPerHunk)
	}
}

func TestSanitizeDiffPreview(t *testing.T) {
	dp := &DiffPreview{
		Files: []DiffFile{
			{Path: ".env", Hunks: []DiffHunk{{Lines: []string{"+SECRET=123"}}}},
			{Path: "id_rsa", Hunks: []DiffHunk{{Lines: []string{"-old key"}}}},
			{Path: "normal.go", Hunks: []DiffHunk{{Lines: []string{"+code"}}}},
			{Path: "cert.pem", Hunks: []DiffHunk{{Lines: []string{"+cert data"}}}},
		},
	}
	sanitizeDiffPreview(dp)
	if len(dp.Files[0].Hunks) != 0 {
		t.Error(".env hunks should be cleared")
	}
	if len(dp.Files[1].Hunks) != 0 {
		t.Error("id_rsa hunks should be cleared")
	}
	if len(dp.Files[2].Hunks) != 1 {
		t.Error("normal.go hunks should be preserved")
	}
	if len(dp.Files[3].Hunks) != 0 {
		t.Error("cert.pem hunks should be cleared")
	}

	// nil input
	sanitizeDiffPreview(nil)
}

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path     string
		sensitive bool
	}{
		{".env", true},
		{"subdir/.env", true},
		{"id_rsa", true},
		{"/root/.ssh/id_ed25519", true},
		{"server.key", true},
		{"cert.pem", true},
		{"credentials.json", true},
		{"config/secrets.yaml", true},
		{"config/secrets.yml", true},
		{"normal.go", false},
		{"README.md", false},
		{"env.go", false},
	}
	for _, tt := range tests {
		if got := isSensitivePath(tt.path); got != tt.sensitive {
			t.Errorf("isSensitivePath(%q) = %v, want %v", tt.path, got, tt.sensitive)
		}
	}
}

func TestContentToAddHunk(t *testing.T) {
	hunks := contentToAddHunk([]string{"line1", "line2"})
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].NewStart != 1 || hunks[0].NewLines != 2 {
		t.Errorf("hunk range wrong: start=%d lines=%d", hunks[0].NewStart, hunks[0].NewLines)
	}

	if contentToAddHunk(nil) != nil {
		t.Error("nil lines should return nil")
	}
	if contentToAddHunk([]string{}) != nil {
		t.Error("empty lines should return nil")
	}
}

func TestBuildDiffFromPatchHunks(t *testing.T) {
	// Valid hunk lines
	hunks, added, removed := buildDiffFromPatchHunks([]string{
		"@@ -1,2 +1,3 @@",
		" unchanged",
		"-removed",
		"+added1",
		"+added2",
	})
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if added != 2 || removed != 1 {
		t.Errorf("added=%d removed=%d, want 2,1", added, removed)
	}

	// Empty input
	hunks2, a, r := buildDiffFromPatchHunks(nil)
	if hunks2 != nil || a != 0 || r != 0 {
		t.Error("nil input should return zeros")
	}

	// Invalid hunk header
	_, _, _ = buildDiffFromPatchHunks([]string{"invalid"})
}

func TestBuildReplaceDiff(t *testing.T) {
	// Single match
	dp := buildReplaceDiff("line1\nline2\nline3\n", "line2", "newline2", false, "test.go")
	if dp == nil {
		t.Fatal("should return diff for valid replace")
	}
	if dp.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", dp.TotalFiles)
	}
	if len(dp.Files[0].Hunks) < 1 {
		t.Error("should have at least 1 hunk")
	}

	// Replace all
	dp2 := buildReplaceDiff("a\na\na\n", "a", "b", true, "test.go")
	if dp2 == nil {
		t.Fatal("replace_all should return diff")
	}
	if dp2.TotalAdded != 3 {
		t.Errorf("replace_all TotalAdded = %d, want 3", dp2.TotalAdded)
	}

	// Not found
	dp3 := buildReplaceDiff("abc\n", "xyz", "123", false, "test.go")
	if dp3 != nil {
		t.Error("not found should return nil")
	}
}

func TestBuildWriteFileDiff(t *testing.T) {
	// New file
	dp := buildWriteFileDiff("", "package main\nfunc main() {}\n", false, "main.go")
	if dp == nil {
		t.Fatal("new file should return diff")
	}
	if dp.Files[0].ChangeType != "add" {
		t.Errorf("new file should be 'add', got %q", dp.Files[0].ChangeType)
	}

	// Overwrite
	dp2 := buildWriteFileDiff("old line\n", "new line\n", true, "main.go")
	if dp2 == nil {
		t.Fatal("overwrite should return diff")
	}
	if dp2.Files[0].ChangeType != "modify" {
		t.Errorf("overwrite should be 'modify', got %q", dp2.Files[0].ChangeType)
	}

	// Empty new file
	dp3 := buildWriteFileDiff("", "", false, "empty.go")
	if dp3 != nil {
		t.Error("empty new file should return nil")
	}
}
