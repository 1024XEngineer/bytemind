package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extensionspkg "bytemind/internal/extensions"
)

func TestRunExtHelpRendersUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunExt([]string{"help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "bytemind ext list") || !strings.Contains(output, "bytemind ext status") {
		t.Fatalf("unexpected help output: %q", output)
	}
}

func TestRunExtNoArgsAndUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := RunExt(nil, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunExt with no args failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "bytemind ext list") {
		t.Fatalf("expected usage output for no-arg run, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	err := RunExt([]string{"unknown"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unknown ext subcommand") {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}
}

func TestRunExtListStatusAndUnload(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	skillDir := filepath.Join(workspace, ".bytemind", "skills", "review")
	writeExtSkillFixture(t, skillDir, "review")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := RunExt([]string{"list", "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunExt list failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "skill.review") {
		t.Fatalf("expected list output to include skill.review, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := RunExt([]string{"status", "skill.review", "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunExt status failed: %v", err)
	}
	statusOutput := stdout.String()
	if !strings.Contains(statusOutput, "id: skill.review") || !strings.Contains(statusOutput, "status: active") {
		t.Fatalf("expected status output to include id and active status, got %q", statusOutput)
	}

	stdout.Reset()
	stderr.Reset()
	if err := RunExt([]string{"unload", "skill.review", "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunExt unload failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "unloaded extension skill.review") {
		t.Fatalf("expected unload confirmation, got %q", stdout.String())
	}
}

func TestRunExtLoadExternalSource(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	externalSource := filepath.Join(t.TempDir(), "docs")
	writeExtSkillFixture(t, externalSource, "docs")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunExt([]string{"load", externalSource, "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunExt load failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "id: skill.docs") || !strings.Contains(output, "status: active") {
		t.Fatalf("expected load output to include skill.docs active, got %q", output)
	}
}

func TestRunExtParsersAndRenderStatuses(t *testing.T) {
	var stderr bytes.Buffer
	workspace, configPath, target, err := parseExtActionTarget(
		"ext status",
		[]string{"--workspace", ".", "--config", "custom.json", "skill.demo"},
		&stderr,
	)
	if err != nil {
		t.Fatalf("parseExtActionTarget failed: %v", err)
	}
	if workspace == "" || configPath != "custom.json" || target != "skill.demo" {
		t.Fatalf("unexpected parseExtActionTarget result: workspace=%q config=%q target=%q", workspace, configPath, target)
	}

	_, _, _, err = parseExtActionTarget("ext status", []string{"--workspace", "."}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "usage: bytemind ext status") {
		t.Fatalf("expected usage error for missing target, got %v", err)
	}

	_, _, err = parseExtCommonFlags("ext list", []string{"--bad-flag"}, &stderr)
	if err == nil {
		t.Fatal("expected parseExtCommonFlags to fail on invalid flag")
	}

	var out bytes.Buffer
	renderExtStatuses(&out, nil)
	if !strings.Contains(out.String(), "no extensions discovered") {
		t.Fatalf("expected empty list render output, got %q", out.String())
	}

	out.Reset()
	renderExtStatuses(&out, []extensionspkg.ExtensionInfo{
		{
			ID:     "skill.b",
			Kind:   extensionspkg.ExtensionSkill,
			Status: extensionspkg.ExtensionStatusActive,
		},
		{
			ID:     "skill.a",
			Kind:   extensionspkg.ExtensionSkill,
			Status: extensionspkg.ExtensionStatusActive,
			Health: extensionspkg.HealthSnapshot{Message: "ok"},
		},
	})
	rendered := out.String()
	if !strings.Contains(rendered, "ID") || !strings.Contains(rendered, "skill.a") || !strings.Contains(rendered, "skill.b") {
		t.Fatalf("expected tabular render output, got %q", rendered)
	}
	if strings.Index(rendered, "skill.a") > strings.Index(rendered, "skill.b") {
		t.Fatalf("expected sorted output by extension id, got %q", rendered)
	}
}

func writeExtSkillFixture(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skill.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
		t.Fatalf("write skill.json failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# /"+name+"\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}
}
