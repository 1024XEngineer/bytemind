package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathRejectsSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	sibling := filepath.Join(base, "repo-other")

	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}

	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ws.ResolvePath(filepath.Join(sibling, "secret.txt"))
	if err == nil {
		t.Fatal("expected sibling path to be rejected")
	}
}

func TestResolvePathAllowsNestedFile(t *testing.T) {
	root := t.TempDir()

	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	path, err := ws.ResolvePath(filepath.Join("nested", "file.txt"))
	if err != nil {
		t.Fatalf("expected nested path to be allowed: %v", err)
	}

	expected := filepath.Join(root, "nested", "file.txt")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

func TestListFilesSkipsBinaryArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "calculator.exe"), []byte{0, 1, 2, 3}, 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	files, err := ws.ListFiles()
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(files, "\n")
	if strings.Contains(joined, "calculator.exe") {
		t.Fatalf("expected executable artifact to be skipped, got %v", files)
	}
	if !strings.Contains(joined, "main.go") {
		t.Fatalf("expected source file to remain visible, got %v", files)
	}
}

func TestGrepFilesSkipsBinaryContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	binaryPayload := append([]byte("match-this-string"), 0, 1, 2, 3)
	if err := os.WriteFile(filepath.Join(root, "artifact.bin"), binaryPayload, 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	results, err := ws.GrepFiles("match-this-string")
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range results {
		if strings.Contains(item, "artifact.bin") {
			t.Fatalf("expected binary file to be ignored by grep, got %v", results)
		}
	}
}

func TestGlobFilesSupportsDoublestarAndSlashPatterns(t *testing.T) {
	root := t.TempDir()
	nestedDir := filepath.Join(root, "pkg", "workspace")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nestedDir, "workspace_test.go")
	if err := os.WriteFile(target, []byte("package workspace"), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	files, err := ws.GlobFiles("**/*.go")
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(files, "\n")
	if !strings.Contains(strings.ReplaceAll(joined, "\\", "/"), "pkg/workspace/workspace_test.go") {
		t.Fatalf("expected doublestar glob to match nested go file, got %v", files)
	}
}
