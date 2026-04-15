package app

import (
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/session"
)

func TestResolveSessionIDSupportsUniquePrefix(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := session.New(`E:\\repo`)
	first.ID = "20260324-120000-abcd"
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}

	second := session.New(`E:\\repo`)
	second.ID = "20260324-130000-efgh"
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveSessionID(store, "20260324-1300")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != second.ID {
		t.Fatalf("expected %q, got %q", second.ID, resolved)
	}
}

func TestResolveSessionIDFailsWhenAmbiguousPrefix(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := session.New(`E:\\repo`)
	a.ID = "20260324-120000-abcd"
	if err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	b := session.New(`E:\\repo`)
	b.ID = "20260324-120100-efgh"
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}

	_, err = ResolveSessionID(store, "20260324-12")
	if err == nil {
		t.Fatal("expected ambiguous prefix error")
	}
	if !strings.Contains(err.Error(), "matched multiple sessions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSessionListLimit(t *testing.T) {
	limit, err := ParseSessionListLimit("")
	if err != nil {
		t.Fatal(err)
	}
	if limit != DefaultSessionListLimit {
		t.Fatalf("expected default limit %d, got %d", DefaultSessionListLimit, limit)
	}

	limit, err = ParseSessionListLimit("12")
	if err != nil {
		t.Fatal(err)
	}
	if limit != 12 {
		t.Fatalf("expected limit 12, got %d", limit)
	}

	if _, err := ParseSessionListLimit("0"); err == nil {
		t.Fatal("expected invalid limit error")
	}
}

func TestSameWorkspaceNormalizesPaths(t *testing.T) {
	workspace := t.TempDir()
	if !SameWorkspace(workspace, filepath.Join(workspace, ".")) {
		t.Fatal("expected normalized paths to match")
	}
}

func TestCreateSessionPersistsSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sess, err := CreateSession(store, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || strings.TrimSpace(sess.ID) == "" {
		t.Fatalf("expected created session with id, got %#v", sess)
	}
	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !SameWorkspace(loaded.Workspace, workspace) {
		t.Fatalf("expected workspace %q, got %q", workspace, loaded.Workspace)
	}
}

func TestResumeSessionInWorkspace(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	target := session.New(filepath.Join(workspace, "."))
	target.ID = "resume-me"
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	resumed, err := ResumeSessionInWorkspace(store, workspace, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != target.ID {
		t.Fatalf("expected resumed id %q, got %q", target.ID, resumed.ID)
	}
}

func TestResumeSessionInWorkspaceRejectsCrossWorkspace(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := session.New(`E:\\repo-a`)
	a.ID = "a"
	if err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	b := session.New(`E:\\repo-b`)
	b.ID = "b"
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}

	_, err = ResumeSessionInWorkspace(store, a.Workspace, "b")
	if err == nil {
		t.Fatal("expected cross-workspace rejection")
	}
	if !strings.Contains(err.Error(), "belongs to workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}
