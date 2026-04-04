package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerLoadsAndResolvesSkillsByAlias(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	if err := os.MkdirAll(filepath.Join(builtin, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builtin, "review", "SKILL.md"), []byte(`---
name: review
description: |
  Review code changes with correctness and risk focus.
allowed-tools: "list_files,read_file,search_text,run_shell"
---
# /review

Use this skill when the user asks for a code review.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(root, builtin, filepath.Join(root, "user"), filepath.Join(root, "project"))
	catalog := manager.Reload()
	if len(catalog.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(catalog.Skills))
	}
	if catalog.Skills[0].ToolPolicy.Policy != ToolPolicyAllowlist {
		t.Fatalf("expected frontmatter allowed-tools to build allowlist policy, got %q", catalog.Skills[0].ToolPolicy.Policy)
	}
	if len(catalog.Skills[0].ToolPolicy.Items) != 4 {
		t.Fatalf("expected 4 allowlist tools, got %#v", catalog.Skills[0].ToolPolicy.Items)
	}

	for _, alias := range []string{"review", "/review"} {
		skill, ok := manager.Find(alias)
		if !ok {
			t.Fatalf("expected alias %q to resolve", alias)
		}
		if skill.Name != "review" {
			t.Fatalf("expected canonical skill name review, got %q", skill.Name)
		}
	}
}

func TestManagerAppliesScopePriority(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	project := filepath.Join(root, "project")
	for _, dir := range []string{
		filepath.Join(builtin, "review"),
		filepath.Join(user, "review"),
		filepath.Join(project, "review"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(builtin, "review", "skill.json"), []byte(`{"name":"review","description":"builtin"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "review", "skill.json"), []byte(`{"name":"review","description":"user"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "skill.json"), []byte(`{"name":"review","description":"project"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(root, builtin, user, project)
	skill, ok := manager.Find("review")
	if !ok {
		t.Fatal("expected review skill to resolve")
	}
	if skill.Description != "project" {
		t.Fatalf("expected project scope to win, got %q", skill.Description)
	}

	catalog := manager.Reload()
	if len(catalog.Overrides) == 0 {
		t.Fatalf("expected override diagnostics, got none")
	}
}

func TestManagerKeepsWorkingWhenManifestIsInvalid(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	if err := os.MkdirAll(filepath.Join(builtin, "bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builtin, "bad", "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(builtin, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(builtin, "good", "skill.json"), []byte(`{"name":"good","description":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManagerWithDirs(root, builtin, filepath.Join(root, "user"), filepath.Join(root, "project"))
	catalog := manager.Reload()
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "good" {
		t.Fatalf("expected valid skill to remain available, got %#v", catalog.Skills)
	}
	if len(catalog.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for invalid skill.json")
	}
	if !strings.Contains(strings.ToLower(catalog.Diagnostics[0].Message), "invalid skill.json") {
		t.Fatalf("unexpected diagnostic: %#v", catalog.Diagnostics[0])
	}
}
