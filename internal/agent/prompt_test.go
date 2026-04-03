package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSystemPromptRendersMainModeSystemAndInstruction(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("Use rg for search before broad shell scans."), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := systemPrompt(PromptInput{
		Workspace:      workspace,
		ApprovalPolicy: "on-request",
		Model:          "gpt-5.4-mini",
		Mode:           "plan",
		Platform:       "linux/amd64",
		Now:            time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Skills: []PromptSkill{
			{Name: "review", Description: "Review code changes for regressions.", Enabled: true},
		},
		Tools:       []string{"read_file", "list_files", "read_file"},
		Instruction: loadAGENTSInstruction(workspace),
	})

	assertContains(t, prompt, "You are ByteMind")
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "plan")
	assertContains(t, prompt, "[Runtime Context]")
	assertContains(t, prompt, "workspace_root: "+workspace)
	assertContains(t, prompt, "platform: linux/amd64")
	assertContains(t, prompt, "date: 2026-04-03")
	assertContains(t, prompt, "model: gpt-5.4-mini")
	assertContains(t, prompt, "mode: plan")
	assertContains(t, prompt, "approval_policy: on-request")
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "- review: Review code changes for regressions. enabled=true")
	assertContains(t, prompt, "[Available Tools]")
	assertContains(t, prompt, "- list_files")
	assertContains(t, prompt, "- read_file")
	assertContains(t, prompt, "[Instructions]")
	assertContains(t, prompt, "Instructions from:")
	assertContains(t, prompt, "Use rg for search before broad shell scans.")
}

func TestSystemPromptOmitsInstructionWhenEmpty(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "never",
		Model:          "deepseek-chat",
		Mode:           "build",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
	})

	assertContains(t, prompt, "[Runtime Context]")
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "- none")
	assertContains(t, prompt, "[Available Tools]")
	assertContains(t, prompt, "- none")
	if strings.Contains(prompt, "[Instructions]") {
		t.Fatalf("did not expect instruction block in prompt: %q", prompt)
	}
}

func TestModePromptDefaultsToBuild(t *testing.T) {
	prompt := strings.TrimSpace(modePrompt(""))
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "build")
}

func TestLoadAGENTSInstruction(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("Always keep edits minimal."), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadAGENTSInstruction(workspace)
	if got == "" {
		t.Fatal("expected AGENTS instruction text, got empty")
	}
	assertContains(t, got, "Instructions from:")
	assertContains(t, got, "Always keep edits minimal.")
}

func TestLoadAGENTSInstructionReturnsEmptyWhenMissing(t *testing.T) {
	if got := loadAGENTSInstruction(t.TempDir()); got != "" {
		t.Fatalf("expected empty instruction text, got %q", got)
	}
}

func TestLoadAGENTSInstructionReturnsEmptyWhenWorkspaceBlank(t *testing.T) {
	if got := loadAGENTSInstruction("   "); got != "" {
		t.Fatalf("expected empty instruction text, got %q", got)
	}
}

func TestLoadAGENTSInstructionReturnsEmptyWhenReadFails(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "AGENTS.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := loadAGENTSInstruction(workspace); got != "" {
		t.Fatalf("expected empty instruction text, got %q", got)
	}
}

func TestFormatToolsDeduplicatesAndSorts(t *testing.T) {
	got := formatTools([]string{"read_file", "list_files", "read_file"})
	want := "- list_files\n- read_file"
	if got != want {
		t.Fatalf("unexpected tool list: got %q want %q", got, want)
	}
}

func TestFormatToolsNone(t *testing.T) {
	if got := formatTools(nil); got != "- none" {
		t.Fatalf("expected \"- none\", got %q", got)
	}
}

func TestFormatSkillsNone(t *testing.T) {
	if got := formatSkills(nil); got != "- none" {
		t.Fatalf("expected \"- none\", got %q", got)
	}
}

func TestIsGitRepository(t *testing.T) {
	workspace := t.TempDir()
	if isGitRepository(workspace) {
		t.Fatalf("expected non-git workspace to be false")
	}
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepository(workspace) {
		t.Fatalf("expected workspace with .git to be true")
	}
}

func TestPromptDebugEnabled(t *testing.T) {
	for _, value := range []string{"1", "true", "yes", "on", "TRUE"} {
		t.Setenv("BYTEMIND_DEBUG_PROMPT", value)
		if !promptDebugEnabled() {
			t.Fatalf("expected debug enabled for value %q", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "no"} {
		t.Setenv("BYTEMIND_DEBUG_PROMPT", value)
		if promptDebugEnabled() {
			t.Fatalf("expected debug disabled for value %q", value)
		}
	}
}

func assertContains(t *testing.T, prompt, needle string) {
	t.Helper()
	if !strings.Contains(prompt, needle) {
		t.Fatalf("expected %q in prompt, got %q", needle, prompt)
	}
}
