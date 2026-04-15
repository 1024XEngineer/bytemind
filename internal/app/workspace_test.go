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
	if !samePath(got, root) {
		t.Fatalf("expected project root %q, got %q", root, got)
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
	if !samePath(got, root) {
		t.Fatalf("expected workspace %q, got %q", root, got)
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

func samePath(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
