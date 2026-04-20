package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerSnapshotDoesNotReload(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".bytemind", "skills", "review")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "skill.json"), []byte(`{"name":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	catalog := mgr.Reload()
	if len(catalog.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(catalog.Skills))
	}
	if err := os.Remove(filepath.Join(project, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	snapshot := mgr.Snapshot()
	if len(snapshot.Skills) != 1 {
		t.Fatalf("expected snapshot to preserve cached skill, got %d", len(snapshot.Skills))
	}
	if snapshot.Skills[0].Instruction == "" {
		t.Fatal("expected snapshot to avoid reload and keep instruction")
	}
}
