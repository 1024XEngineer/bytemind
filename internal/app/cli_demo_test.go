package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDemoUnknownDemo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunDemo([]string{"nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown demo")
	}
	if !strings.Contains(stderr.String(), "Unknown demo") {
		t.Errorf("expected 'Unknown demo', got %s", stderr.String())
	}
}

func TestFindProjectRootFromProject(t *testing.T) {
	root := findProjectRoot()
	if root == "" {
		t.Fatal("expected non-empty project root")
	}
	// Should be the bytemind project root (contains go.mod)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("project root should contain go.mod: %v", err)
	}
}

func TestFindProjectRootFromNonProjectDir(t *testing.T) {
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origWd)

	root := findProjectRoot()
	if root != "" {
		t.Fatalf("expected empty for non-project dir, got %s", root)
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dst = filepath.Join(dst, "sub")

	// Create source files and subdirectory
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(src, "subdir"), 0o755)
	os.WriteFile(filepath.Join(src, "subdir", "b.txt"), []byte("world"), 0o644)

	err := copyDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	// Verify files were copied
	data1, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil { t.Fatal(err) }
	if string(data1) != "hello" { t.Fatalf("expected 'hello', got %s", string(data1)) }

	data2, err := os.ReadFile(filepath.Join(dst, "subdir", "b.txt"))
	if err != nil { t.Fatal(err) }
	if string(data2) != "world" { t.Fatalf("expected 'world', got %s", string(data2)) }
}
