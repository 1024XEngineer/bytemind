package subagents

import "testing"

func TestParseFrontmatterFieldsTreatsEmptyValueAsMultiline(t *testing.T) {
	fields := parseFrontmatterFields(`
description:
  line one
  line two
mode: build
`)
	if got := fields["description"]; got != "line one\nline two" {
		t.Fatalf("expected multiline description expansion, got %q", got)
	}
	if got := fields["mode"]; got != "build" {
		t.Fatalf("expected mode build, got %q", got)
	}
}

func TestParseFrontmatterMarkdownUsesSharedParserWithSubAgentOptions(t *testing.T) {
	fields, body := parseFrontmatterMarkdown(`---
name: explorer
description:
  repo scanner
---
scan repo
`)
	if got := fields["name"]; got != "explorer" {
		t.Fatalf("expected name explorer, got %q", got)
	}
	if got := fields["description"]; got != "repo scanner" {
		t.Fatalf("expected multiline description, got %q", got)
	}
	if body != "scan repo" {
		t.Fatalf("expected markdown body preserved, got %q", body)
	}
}
