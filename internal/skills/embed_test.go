package skills

import (
	"io/fs"
	"testing"
)

func TestLoadBuiltinFromEmbeddedReturnsAllSkills(t *testing.T) {
	skills, diags := loadBuiltinFromEmbedded(ScopeBuiltin)
	if len(skills) == 0 {
		t.Fatal("expected at least one builtin skill from embedded FS")
	}
	names := make(map[string]bool, len(skills))
	for _, s := range skills {
		if s.Name == "" {
			t.Errorf("skill has empty name")
		}
		if s.Description == "" {
			t.Errorf("skill %q has empty description", s.Name)
		}
		if s.Scope != ScopeBuiltin {
			t.Errorf("skill %q has scope %q, want %q", s.Name, s.Scope, ScopeBuiltin)
		}
		if s.Entry.Slash == "" {
			t.Errorf("skill %q has empty entry slash", s.Name)
		}
		names[s.Name] = true
	}
	if diags != nil {
		for _, d := range diags {
			t.Logf("diagnostic: %s %s %s - %s", d.Level, d.Skill, d.Path, d.Message)
		}
	}
	if !names["review"] {
		t.Errorf("expected review skill to be loaded from embedded FS, got: %v", names)
	}
	if !names["bug-investigation"] {
		t.Errorf("expected bug-investigation skill to be loaded from embedded FS")
	}
}

func TestLoadSkillsFromFSWithEmbeddedFSPathSeparators(t *testing.T) {
	skills, _ := loadSkillsFromFS(ScopeBuiltin, builtinFS, ".")
	if len(skills) == 0 {
		t.Fatal("expected skills when loading from embedded FS with root '.'")
	}

	subdir, err := fs.ReadDir(builtinFS, "review")
	if err != nil {
		t.Fatalf("cant read review subdirectory from embedded FS: %v", err)
	}
	if len(subdir) == 0 {
		t.Fatal("expected files in review embedded dir")
	}
}
