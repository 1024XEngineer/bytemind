package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerLoadReadsWorkspaceSkills(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: review
description: Review code carefully
---
# Review

Check risks before summarizing.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(workspace)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}

	skill := manager.Get("review")
	if skill == nil {
		t.Fatal("expected review skill to be loaded")
	}
	if skill.Description != "Review code carefully" {
		t.Fatalf("unexpected description: %q", skill.Description)
	}
	if skill.Instructions() == "" || skill.Instructions() == skill.Content {
		t.Fatalf("expected front matter to be stripped from instructions, got %q", skill.Instructions())
	}
}

func TestManagerLoadIgnoresMissingSkillsDir(t *testing.T) {
	manager := NewManager(t.TempDir())
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("expected no skills, got %d", len(got))
	}
}

func TestSkillDisplayDescriptionUsesChineseOverride(t *testing.T) {
	skill := &Skill{
		Name:        "pdf",
		Description: "Use this skill whenever the user wants to do anything with PDF files.",
	}

	if got := skill.DisplayDescription(); got != "PDF 提取、合并、OCR、拆分" {
		t.Fatalf("unexpected display description: %q", got)
	}
}

func TestSkillDisplayDescriptionFallsBackToShortText(t *testing.T) {
	skill := &Skill{
		Name:        "custom",
		Description: "A very long custom description that should be shortened for display.",
	}

	got := skill.DisplayDescription()
	if got == "" {
		t.Fatal("expected non-empty display description")
	}
	if len([]rune(got)) > 18 {
		t.Fatalf("expected compact fallback description, got %q", got)
	}
}

func TestExtractSkillNameTrimsQuotedValue(t *testing.T) {
	got := extractSkillName("name: \"screenshot\"\n", "fallback")
	if got != "screenshot" {
		t.Fatalf("expected trimmed skill name, got %q", got)
	}
}

func TestBuiltinSkillAuthorExists(t *testing.T) {
	skill := Builtin(BuiltinSkillAuthorName)
	if skill == nil {
		t.Fatal("expected builtin skill-author to exist")
	}
	if skill.DisplayDescription() != "按需求生成或改写项目 skill" {
		t.Fatalf("unexpected builtin display description: %q", skill.DisplayDescription())
	}
}
