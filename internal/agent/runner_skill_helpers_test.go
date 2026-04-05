package agent

import (
	"os"
	"path/filepath"
	"testing"

	"bytemind/internal/session"
	"bytemind/internal/skills"
)

func TestRunnerListSkillsAndGetActiveSkillBranches(t *testing.T) {
	runnerWithoutManager := NewRunner(Options{
		Workspace:    t.TempDir(),
		SkillManager: nil,
	})
	if skillsList, diagnostics := runnerWithoutManager.ListSkills(); len(skillsList) != 0 || len(diagnostics) != 0 {
		t.Fatalf("expected empty list/diagnostics with default manager, got list=%v diags=%v", skillsList, diagnostics)
	}
	if _, ok := runnerWithoutManager.GetActiveSkill(nil); ok {
		t.Fatal("expected no active skill for nil session")
	}
	if _, ok := runnerWithoutManager.GetActiveSkill(&session.Session{}); ok {
		t.Fatal("expected no active skill for session without active skill")
	}

	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	skillDir := filepath.Join(builtinDir, "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# review\nReview code changes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	runner := NewRunner(Options{
		Workspace:    workspace,
		SkillManager: manager,
	})
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	active, ok := runner.GetActiveSkill(sess)
	if !ok {
		t.Fatal("expected active skill to resolve")
	}
	if active.Name != "review" {
		t.Fatalf("expected review skill, got %+v", active)
	}
}
