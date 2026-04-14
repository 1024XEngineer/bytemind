package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSkillMDContent(t *testing.T) {
	content := strings.Join([]string{
		"\ufeff---",
		"name: sample-skill",
		"description: \"Do sample things\"",
		"compatibility: |-",
		"  requires: none",
		"---",
		"# Header",
		"Body",
	}, "\n")

	doc, err := parseSkillMDContent(content)
	if err != nil {
		t.Fatalf("parseSkillMDContent returned error: %v", err)
	}
	if doc.Name != "sample-skill" {
		t.Fatalf("unexpected name: %q", doc.Name)
	}
	if doc.Description != "Do sample things" {
		t.Fatalf("unexpected description: %q", doc.Description)
	}
	if got := doc.Frontmatter["compatibility"]; got != "requires: none" {
		t.Fatalf("unexpected compatibility: %q", got)
	}
}

func TestParseSkillMDContentErrors(t *testing.T) {
	cases := []string{
		"name: x\ndescription: y",
		"---\nname: x\ndescription: y",
	}
	for i, c := range cases {
		_, err := parseSkillMDContent(c)
		if err == nil {
			t.Fatalf("case %d expected error, got nil", i)
		}
	}
}

func TestParseSimpleFrontmatter(t *testing.T) {
	lines := []string{
		"name: test-skill",
		"description: \"quoted\"",
		"compatibility: |",
		"  uses-go",
		"  no-python",
		"metadata: ignored",
	}
	fm := parseSimpleFrontmatter(lines)
	if fm["name"] != "test-skill" {
		t.Fatalf("unexpected name: %q", fm["name"])
	}
	if fm["description"] != "quoted" {
		t.Fatalf("unexpected description: %q", fm["description"])
	}
	if fm["compatibility"] != "uses-go no-python" {
		t.Fatalf("unexpected compatibility block value: %q", fm["compatibility"])
	}
}

func TestTrimQuotes(t *testing.T) {
	if got := trimQuotes(`"hello"`); got != "hello" {
		t.Fatalf("trimQuotes failed: %q", got)
	}
	if got := trimQuotes(`'world'`); got != "world" {
		t.Fatalf("trimQuotes failed: %q", got)
	}
	if got := trimQuotes(`plain`); got != "plain" {
		t.Fatalf("trimQuotes failed: %q", got)
	}
}

func TestReadWriteJSONFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "data.json")
	src := map[string]any{"ok": true, "n": 3}
	if err := writeJSONFile(path, src); err != nil {
		t.Fatalf("writeJSONFile: %v", err)
	}

	var dst map[string]any
	if err := readJSONFile(path, &dst); err != nil {
		t.Fatalf("readJSONFile: %v", err)
	}
	if asBoolDefault(dst["ok"], false) != true {
		t.Fatalf("unexpected ok: %#v", dst["ok"])
	}
	if int(asFloat(dst["n"])) != 3 {
		t.Fatalf("unexpected n: %#v", dst["n"])
	}

	emptyPath := filepath.Join(tmp, "empty.json")
	if err := os.WriteFile(emptyPath, []byte(" \n\t "), 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	if err := readJSONFile(emptyPath, &dst); err == nil {
		t.Fatalf("expected error for empty json file")
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("SKILL_BOOL_ENV", "true")
	if !parseBoolEnv("SKILL_BOOL_ENV") {
		t.Fatalf("expected true")
	}
	t.Setenv("SKILL_BOOL_ENV", "0")
	if parseBoolEnv("SKILL_BOOL_ENV") {
		t.Fatalf("expected false")
	}
}

func TestFindProjectRootWithClaudeDir(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".claude", "keep"), "x")
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	withWorkingDir(t, deep)
	got := findProjectRootWithClaudeDir()
	if got != root {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}

func TestMustAbs(t *testing.T) {
	got := mustAbs(".")
	if !filepath.IsAbs(got) {
		t.Fatalf("mustAbs should return absolute path, got %q", got)
	}
	if got := mustAbs("  "); got != "  " {
		t.Fatalf("empty-ish path should be returned as-is")
	}
}

func TestParseSkillMDDir(t *testing.T) {
	tmp := t.TempDir()
	skillDir := createMinimalSkill(t, tmp, "dir-skill", "desc")
	doc, err := parseSkillMDDir(skillDir)
	if err != nil {
		t.Fatalf("parseSkillMDDir: %v", err)
	}
	want := skillDoc{
		Name:        "dir-skill",
		Description: "desc",
	}
	if doc.Name != want.Name || doc.Description != want.Description {
		t.Fatalf("unexpected doc: %+v", doc)
	}
}

func TestExtractTopLevelKeys(t *testing.T) {
	lines := []string{
		"name: a",
		"description: b",
		"  nested: ignored",
		"metadata: c",
		"name: duplicate",
	}
	got := extractTopLevelKeys(lines)
	want := []string{"name", "description", "metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected keys: got=%v want=%v", got, want)
	}
}
