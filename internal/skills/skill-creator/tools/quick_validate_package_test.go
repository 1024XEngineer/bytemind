package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkill(t *testing.T) {
	tmp := t.TempDir()
	validDir := createMinimalSkill(t, tmp, "valid-skill", "valid description")
	res := validateSkill(validDir)
	if !res.Valid {
		t.Fatalf("expected valid skill, got: %s", res.Message)
	}
}

func TestValidateSkillErrors(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	res := validateSkill(skillDir)
	if res.Valid || !strings.Contains(res.Message, "SKILL.md not found") {
		t.Fatalf("expected missing SKILL.md error, got: %+v", res)
	}

	writeTestFile(t, filepath.Join(skillDir, "SKILL.md"), strings.Join([]string{
		"---",
		"name: Bad_Name",
		"description: desc",
		"bad-key: x",
		"---",
		"# body",
	}, "\n"))
	res = validateSkill(skillDir)
	if res.Valid || !strings.Contains(res.Message, "Unexpected key(s)") {
		t.Fatalf("expected unexpected key error, got: %+v", res)
	}
}

func TestRunQuickValidateUsage(t *testing.T) {
	if err := runQuickValidate(nil); err == nil {
		t.Fatalf("expected usage error")
	}
}

func TestPackageSkill(t *testing.T) {
	tmp := t.TempDir()
	skillDir := createMinimalSkill(t, tmp, "pack-skill", "pack desc")
	writeTestFile(t, filepath.Join(skillDir, "README.md"), "hello")
	writeTestFile(t, filepath.Join(skillDir, "evals", "should_skip.txt"), "skip")
	writeTestFile(t, filepath.Join(skillDir, "node_modules", "a.txt"), "skip")
	writeTestFile(t, filepath.Join(skillDir, ".DS_Store"), "skip")

	outDir := filepath.Join(tmp, "out")
	outPath, err := packageSkill(skillDir, outDir)
	if err != nil {
		t.Fatalf("packageSkill: %v", err)
	}
	if !strings.HasSuffix(outPath, ".skill") {
		t.Fatalf("unexpected output path: %s", outPath)
	}

	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "pack-skill/SKILL.md") {
		t.Fatalf("zip missing SKILL.md: %v", names)
	}
	if strings.Contains(joined, "pack-skill/evals/should_skip.txt") {
		t.Fatalf("zip should exclude evals dir: %v", names)
	}
	if strings.Contains(joined, "pack-skill/node_modules/a.txt") {
		t.Fatalf("zip should exclude node_modules: %v", names)
	}
	if strings.Contains(joined, ".DS_Store") {
		t.Fatalf("zip should exclude .DS_Store: %v", names)
	}
}

func TestShouldExcludePath(t *testing.T) {
	if !shouldExcludePath("x/node_modules/a.txt") {
		t.Fatalf("node_modules should be excluded")
	}
	if !shouldExcludePath("x/evals/a.txt") {
		t.Fatalf("root evals should be excluded")
	}
	if !shouldExcludePath("x/.DS_Store") {
		t.Fatalf(".DS_Store should be excluded")
	}
	if shouldExcludePath("x/src/a.txt") {
		t.Fatalf("regular files should not be excluded")
	}
}

func TestRunPackageSkillUsage(t *testing.T) {
	if err := runPackageSkill(nil); err == nil {
		t.Fatalf("expected usage error")
	}
}
