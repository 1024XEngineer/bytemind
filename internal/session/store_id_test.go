package session

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeSessionIDRejectsTraversalAndAbsoluteForms(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"   ",
		".",
		"..",
		"../escape",
		`..\escape`,
		"a/b",
		`a\b`,
		filepath.Join(string(filepath.Separator), "abs", "id"),
	}
	for _, raw := range cases {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := normalizeSessionID(raw); err == nil {
				t.Fatalf("expected %q to be rejected", raw)
			}
		})
	}

	if runtime.GOOS == "windows" {
		if _, err := normalizeSessionID(`C:escape`); err == nil {
			t.Fatal("expected windows volume-prefixed session id to be rejected")
		}
	}
}

func TestPathForWorkspaceSessionRejectsInvalidID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pathForWorkspaceSession(t.TempDir(), "../escape"); err == nil {
		t.Fatal("expected invalid session id to be rejected")
	}
}

func TestStoreSaveRejectsSessionIDPathTraversal(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := New(t.TempDir())
	sess.ID = "../escape"
	if err := store.Save(sess); err == nil {
		t.Fatal("expected Save to reject invalid session id")
	}
}

func TestSameWorkspacePathCaseRulesFollowPlatform(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "RepoCase")
	right := filepath.Join(root, strings.ToLower(filepath.Base(left)))

	got := sameWorkspacePath(left, right)
	if runtime.GOOS == "windows" {
		if !got {
			t.Fatal("expected windows path comparison to be case-insensitive")
		}
		return
	}
	if got {
		t.Fatal("expected non-windows path comparison to be case-sensitive")
	}
}
