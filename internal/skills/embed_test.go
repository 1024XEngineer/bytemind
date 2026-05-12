package skills

import (
	"io/fs"
	"os"
	"path/filepath"
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

func TestLoadSkillsFromFSWithNonExistentRoot(t *testing.T) {
	skills, diags := loadSkillsFromFS(ScopeBuiltin, builtinFS, "nonexistent")
	if len(skills) != 0 {
		t.Fatalf("expected no skills for non-existent root, got %d", len(skills))
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for non-existent root, got %d", len(diags))
	}
}

func TestLoadSkillFromFSDirWithMissingFiles(t *testing.T) {
	skill, ok, diags := loadSkillFromFSDir(ScopeBuiltin, builtinFS, "nonexistent", "test")
	if ok {
		t.Fatal("expected ok=false for non-existent directory")
	}
	if skill.Name != "" {
		t.Fatalf("expected empty skill for non-existent dir")
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for non-existent dir")
	}
}

func TestFileExistsFS(t *testing.T) {
	if !fileExistsFS(builtinFS, "review/skill.json") {
		t.Error("expected review/skill.json to exist")
	}
	if fileExistsFS(builtinFS, "nonexistent/file.json") {
		t.Error("expected nonexistent/file.json to not exist")
	}
	if fileExistsFS(builtinFS, ".") {
		t.Error("expected root dir to not be a file")
	}
}

func TestUseEmbeddedBuiltinsLoadsBuiltinSkills(t *testing.T) {
	root := t.TempDir()
	m := NewManagerWithDirs(root, filepath.Join(root, "nonexistent"), "", "")
	m.UseEmbeddedBuiltins()
	catalog := m.Reload()
	if len(catalog.Skills) == 0 {
		t.Fatal("expected embedded builtin skills after UseEmbeddedBuiltins with non-existent dir")
	}
	names := make(map[string]bool)
	for _, s := range catalog.Skills {
		names[s.Name] = true
	}
	if !names["review"] {
		t.Errorf("expected review skill, got: %v", names)
	}
	if !names["bug-investigation"] {
		t.Errorf("expected bug-investigation skill")
	}
}

func TestUseEmbeddedBuiltinsDoesNotOverrideExistingBuiltinDir(t *testing.T) {
	root := t.TempDir()
	builtinDir := filepath.Join(root, "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManagerWithDirs(root, builtinDir, "", "")
	m.UseEmbeddedBuiltins()
	catalog := m.Reload()
	if len(catalog.Skills) != 0 {
		t.Fatalf("expected no skills when builtin dir exists but is empty, got %d", len(catalog.Skills))
	}
}

func TestUseEmbeddedBuiltinsRespectsPriority(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, ".bytemind", "skills", "review")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "skill.json"), []byte(`{"name":"review","description":"project review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManagerWithDirs(root, filepath.Join(root, "nonexistent"), "", filepath.Join(root, ".bytemind", "skills"))
	m.UseEmbeddedBuiltins()
	catalog := m.Reload()
	names := make(map[string]int)
	for _, s := range catalog.Skills {
		names[s.Name]++
	}
	if names["review"] != 1 {
		t.Fatalf("expected review to appear once (project overrides builtin), but got count=%d, names=%v", names["review"], names)
	}
}

func TestLoadSkillsFromFSWithOSDirFS(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"my-skill","description":"test skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n# My Skill\nDo something."), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, diags := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d: %v", len(skills), diags)
	}
	if skills[0].Name != "my-skill" {
		t.Fatalf("expected skill name my-skill, got %q", skills[0].Name)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestLoadSkillsFromFSWithInvalidJSON(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, diags := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills for invalid JSON, got %d", len(skills))
	}
	found := false
	for _, d := range diags {
		if d.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error diagnostic for invalid JSON, got %v", diags)
	}
}

func TestFileExistsFSOnDirectory(t *testing.T) {
	if fileExistsFS(builtinFS, "review") {
		t.Error("expected review directory to not be a file")
	}
}

func TestLoadSkillFromFSDirWithSkillOnly(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "noskilljson")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: from-skill\n---\n# Body"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, diags := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 || skills[0].Name != "from-skill" {
		t.Fatalf("expected skill named from-skill, got %+v", skills)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestLoadSkillFromFSDirWithNameFromDirName(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "dir-name-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"description":"no name in manifest"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, diags := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 || skills[0].Name != "dir-name-skill" {
		t.Fatalf("expected skill name from dir, got %+v", skills)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestLoadSkillFromFSDirWithToolPolicyDiagnostic(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "bad-policy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"bad-policy","tools":{"policy":"invalid"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, diags := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	found := false
	for _, d := range diags {
		if d.Level == "warn" && d.Skill == "bad-policy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warning diagnostic for invalid tool policy, got %v", diags)
	}
}

func TestLoadSkillFromFSDirWithTitle(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "titled")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"titled","title":"My Title","description":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, _ := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 || skills[0].Title != "My Title" {
		t.Fatalf("expected title 'My Title', got %+v", skills)
	}
}

func TestLoadSkillFromFSDirWithEntrySlash(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "aliased")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"aliased","description":"test","entry":{"slash":"custom-slash"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, _ := loadSkillsFromFS(ScopeBuiltin, os.DirFS(root), ".")
	if len(skills) != 1 || skills[0].Entry.Slash != "/custom-slash" {
		t.Fatalf("expected entry slash '/custom-slash', got %+v", skills[0].Entry)
	}
}

func TestUseEmbeddedBuiltinsIsNoopWhenBuiltinDirExists(t *testing.T) {
	root := t.TempDir()
	builtinDir := filepath.Join(root, "builtin")
	customDir := filepath.Join(builtinDir, "custom")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "skill.json"), []byte(`{"name":"custom","description":"custom skill"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManagerWithDirs(root, builtinDir, "", "")
	m.UseEmbeddedBuiltins()
	catalog := m.Reload()
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "custom" {
		t.Fatalf("expected only custom skill (not embedded), got %+v", catalog.Skills)
	}
}
