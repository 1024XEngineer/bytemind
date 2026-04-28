package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProjectRootFindsAncestorMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got := DetectProjectRoot(nested)
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty project root")
	}
	if !hasProjectMarker(got) {
		t.Fatalf("expected detected root %q to contain project marker", got)
	}
	if !pathWithinRoot(nested, got) {
		t.Fatalf("expected nested path %q to be within detected root %q", nested, got)
	}
}

func TestResolveWorkspaceAutoDetectsProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "pkg", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty workspace")
	}
	if !hasProjectMarker(got) {
		t.Fatalf("expected workspace %q to contain project marker", got)
	}
	if !pathWithinRoot(nested, got) {
		t.Fatalf("expected cwd %q to be within workspace %q", nested, got)
	}
}

func TestIsBroadWorkspacePathWithHomeFlagsKnownBroadRoots(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	for _, dir := range []string{
		home,
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Downloads"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if !IsBroadWorkspacePathWithHome(dir, home) {
			t.Fatalf("expected %q to be treated as broad workspace", dir)
		}
	}
}

func TestIsBroadWorkspacePathWithHomeFlagsLargeDirectory(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < DefaultBroadWorkspaceEntryThreshold; i++ {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("entry-%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if !IsBroadWorkspacePathWithHome(dir, "") {
		t.Fatalf("expected directory with %d entries to be broad", DefaultBroadWorkspaceEntryThreshold)
	}
}

func TestIsBroadWorkspacePathWithHomeAllowsSmallDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if IsBroadWorkspacePathWithHome(dir, "") {
		t.Fatalf("did not expect %q to be treated as broad workspace", dir)
	}
}

func TestResolveWorkspaceRejectsFilePathOverride(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveWorkspace(file)
	if err == nil {
		t.Fatal("expected non-directory workspace override to fail")
	}
	if !strings.Contains(err.Error(), "workspace must be a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func normalizeExistingPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(resolved)
	}
	return abs
}

func pathWithinRoot(path, root string) bool {
	path = normalizeExistingPath(path)
	root = normalizeExistingPath(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
