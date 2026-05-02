package skills

import "testing"

func TestParseFrontmatterFieldsKeepsEmptyValueWithoutMultilineExpansion(t *testing.T) {
	fields := parseFrontmatterFields(`
description:
  line one
mode: build
`)
	if got := fields["description"]; got != "" {
		t.Fatalf("expected empty description without multiline expansion, got %q", got)
	}
	if got := fields["mode"]; got != "build" {
		t.Fatalf("expected mode build, got %q", got)
	}
}

func TestParseFrontmatterMarkdownUsesSharedParser(t *testing.T) {
	fields, body := parseFrontmatterMarkdown(`---
name: reviewer
description: "skill description"
---
review body
`)
	if got := fields["name"]; got != "reviewer" {
		t.Fatalf("expected name reviewer, got %q", got)
	}
	if got := fields["description"]; got != "skill description" {
		t.Fatalf("expected trimmed description, got %q", got)
	}
	if body != "review body" {
		t.Fatalf("expected markdown body preserved, got %q", body)
	}
}
