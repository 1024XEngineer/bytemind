package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
)

func TestRunSafetyStatusOutput(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	expected := []string{
		"ByteMind Safety Report",
		"Approval policy",
		"Approval mode",
		"Writable roots",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("expected safety status to contain %q", s)
		}
	}
}

func TestRunSafetyExplainOutput(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	expected := []string{
		"ByteMind Safety Model",
		"Tool Safety Classes",
		"Approval Policy",
		"Sandbox",
		"Writable Roots",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("expected safety explain to contain %q", s)
		}
	}
}

func TestRunSafetyDefaultShowsStatus(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Report") {
		t.Errorf("expected default safety to show status, got %s", output[:60])
	}
}

func TestRunSafetyStatusWithFlagsAfterSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	// This was broken: "status -workspace <path>" would ignore -workspace
	err := RunSafety([]string{"status", "-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Report") {
		t.Errorf("expected safety status output, got %s", output[:60])
	}
}

func TestRunSafetyExplainWithFlagsAfterSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain", "-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Model") {
		t.Errorf("expected safety explain output, got %s", output[:60])
	}
}

func TestRunSafetyStatusContainsShelAllowlist(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Shell allowlist") {
		t.Errorf("expected Shell allowlist section, got %s", output[:100])
	}
}

func TestRunSafetyStatusContainsAccessSummary(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Access summary") {
		t.Errorf("expected Access summary section, got %s", output[:100])
	}
}

func TestRunSafetyExplainContainsWhatIsNotProtected(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "NOT protected") {
		t.Errorf("expected 'NOT protected' section, got %s", output[:200])
	}
}

func TestRunSafetyExplainContainsEmoji(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "\u2705") {
		t.Errorf("expected checkmark emoji in safety explain output")
	}
}

func TestRunSafetyReportContainsBlockedCommands(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Blocked commands") {
		t.Errorf("expected safety report to contain blocked commands, got %s", output[:100])
	}
}

func TestRunSafetyReportContainsWorkspace(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status", "-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Workspace:") {
		t.Errorf("expected safety report to contain Workspace, got %s", output[:100])
	}
}

func TestSafetyReportFullAccessBranch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := configpkg.Default(dir)
	cfg.ApprovalMode = "full_access"
	cfg.SandboxEnabled = true
	cfg.SystemSandboxMode = "best_effort"
	cfg.WritableRoots = []string{dir}
	cfg.ExecAllowlist = []configpkg.ExecAllowRule{{Command: "go", ArgsPattern: []string{"test"}}}
	cfg.NetworkAllowlist = []configpkg.NetworkAllowRule{{Host: "example.com", Port: 443, Scheme: "https"}}
	configpkg.WriteConfig(cfgPath, cfg)

	var stdout bytes.Buffer
	err := RunSafety([]string{"status", "-config", cfgPath}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{"full_access", "Sandbox: enabled (best_effort)", "Writable roots",
		"Blocked commands", "Allowlist: https://example.com:443", "go test", "auto-approved"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

func TestSafetyReportNeverPolicyBranch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := configpkg.Default(dir)
	cfg.ApprovalPolicy = "never"
	configpkg.WriteConfig(cfgPath, cfg)

	var stdout bytes.Buffer
	err := RunSafety([]string{"status", "-config", cfgPath}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "approval_policy=never") {
		t.Errorf("expected approval_policy=never in output, got %s", output[:200])
	}
}
