package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerAuthorCreatesProjectSkillScaffold(t *testing.T) {
	root := t.TempDir()
	manager := NewManagerWithDirs(
		root,
		filepath.Join(root, "builtin"),
		filepath.Join(root, "user"),
		filepath.Join(root, "project"),
	)

	result, err := manager.Author("review-plus", ScopeProject, "Review code changes and report regressions.")
	if err != nil {
		t.Fatalf("author failed: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created=true, got %#v", result)
	}
	if _, err := os.Stat(result.ManifestPath); err != nil {
		t.Fatalf("expected manifest to exist: %v", err)
	}
	if _, err := os.Stat(result.SkillPath); err != nil {
		t.Fatalf("expected skill markdown to exist: %v", err)
	}

	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(manifestData)
	if !strings.Contains(manifest, `"name": "review-plus"`) {
		t.Fatalf("expected manifest name, got %q", manifest)
	}
	if !strings.Contains(manifest, `"description": "Review code changes and report regressions."`) {
		t.Fatalf("expected brief to populate description, got %q", manifest)
	}

	skillData, err := os.ReadFile(result.SkillPath)
	if err != nil {
		t.Fatal(err)
	}
	skillText := string(skillData)
	if !strings.Contains(skillText, "when_to_use: |") {
		t.Fatalf("expected when_to_use frontmatter, got %q", skillText)
	}
	if !strings.Contains(skillText, "Review code changes and report regressions.") {
		t.Fatalf("expected brief in skill markdown, got %q", skillText)
	}

	skill, ok := manager.Find("/review-plus")
	if !ok {
		t.Fatalf("expected authored skill to be discoverable")
	}
	if skill.Name != "review-plus" {
		t.Fatalf("unexpected authored skill name: %#v", skill)
	}
}

func TestManagerAuthorUpdatesExistingSkillAndKeepsBody(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	skillDir := filepath.Join(project, "custom-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "custom-skill",
  "description": "old description",
  "entry": {"slash": "/custom-skill"},
  "tools": {"policy": "inherit"}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: custom-skill
description: old description
---

# custom-skill

## Existing Body

Keep this body.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(
		root,
		filepath.Join(root, "builtin"),
		filepath.Join(root, "user"),
		project,
	)

	result, err := manager.Author("custom-skill", ScopeProject, "Handle recurring repository triage tasks.")
	if err != nil {
		t.Fatalf("author update failed: %v", err)
	}
	if result.Created {
		t.Fatalf("expected update on existing skill, got created=true")
	}
	if !result.Updated {
		t.Fatalf("expected updated=true, got %#v", result)
	}

	skillData, err := os.ReadFile(result.SkillPath)
	if err != nil {
		t.Fatal(err)
	}
	skillText := string(skillData)
	if !strings.Contains(skillText, "## Existing Body") {
		t.Fatalf("expected existing body to be kept, got %q", skillText)
	}
	if !strings.Contains(skillText, "Handle recurring repository triage tasks.") {
		t.Fatalf("expected brief to update skill frontmatter, got %q", skillText)
	}
}

func TestManagerAuthorRejectsInvalidName(t *testing.T) {
	root := t.TempDir()
	manager := NewManagerWithDirs(
		root,
		filepath.Join(root, "builtin"),
		filepath.Join(root, "user"),
		filepath.Join(root, "project"),
	)

	if _, err := manager.Author("bad name with spaces", ScopeProject, ""); err == nil {
		t.Fatalf("expected invalid name error")
	}
}

func TestManagerClearRemovesProjectSkill(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	skillDir := filepath.Join(project, "review-plus")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"review-plus","description":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# review-plus"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(
		root,
		filepath.Join(root, "builtin"),
		filepath.Join(root, "user"),
		project,
	)

	result, err := manager.Clear("/review-plus")
	if err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	if !result.Removed {
		t.Fatalf("expected removed=true, got %#v", result)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("expected skill dir removed, stat err=%v", err)
	}
	if _, ok := manager.Find("review-plus"); ok {
		t.Fatalf("expected cleared skill to disappear from catalog")
	}
}

func TestManagerClearRejectsBuiltinSkill(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	if err := os.MkdirAll(filepath.Join(builtin, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builtin, "review", "skill.json"), []byte(`{"name":"review","description":"builtin"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(
		root,
		builtin,
		filepath.Join(root, "user"),
		filepath.Join(root, "project"),
	)

	if _, err := manager.Clear("review"); err == nil {
		t.Fatalf("expected builtin clear to fail")
	}
}
