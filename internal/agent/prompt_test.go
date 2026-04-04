package agent

import (
	"fmt"
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
		Tools: []string{"read_file", "list_files", "read_file"},
		ActiveSkill: &PromptActiveSkill{
			Name:         "review",
			Description:  "Review code changes with correctness focus.",
			WhenToUse:    "When user asks for review.",
			Instructions: "Prioritize regressions and missing tests.",
			Args: map[string]string{
				"base_ref": "main",
			},
			ToolPolicy: "allowlist",
			Tools:      []string{"read_file", "search_text"},
		},
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
	assertContains(t, prompt, "Skills are user-selected session profiles")
	assertContains(t, prompt, "- review: Review code changes for regressions. enabled=true")
	assertContains(t, prompt, "[Available Tools]")
	assertContains(t, prompt, "- list_files")
	assertContains(t, prompt, "- read_file")
	assertContains(t, prompt, "[Active Skill]")
	assertContains(t, prompt, "Tool Policy: allowlist")
	assertContains(t, prompt, "[Instructions]")
	assertContains(t, prompt, "Instructions from:")
	assertContains(t, prompt, "Use rg for search before broad shell scans.")
	assertNoTemplateMarkers(t, prompt)
}

func TestSystemPromptOmitsOptionalBlocksWhenEmpty(t *testing.T) {
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
	if strings.Contains(prompt, "[Active Skill]") {
		t.Fatalf("did not expect active skill block in prompt: %q", prompt)
	}
	assertNoTemplateMarkers(t, prompt)
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

func TestFormatSkillsLimitsAndSummarizesOverflow(t *testing.T) {
	skills := make([]PromptSkill, 0, maxPromptSkillEntries+2)
	for i := 0; i < maxPromptSkillEntries+2; i++ {
		skills = append(skills, PromptSkill{
			Name:        fmt.Sprintf("skill-%02d", i),
			Description: strings.Repeat("x", maxPromptSkillDescriptionRune+20),
			Enabled:     true,
		})
	}
	got := formatSkills(skills)
	if !strings.Contains(got, "- ... and 2 more skill(s)") {
		t.Fatalf("expected overflow summary line, got %q", got)
	}
	if strings.Contains(got, strings.Repeat("x", maxPromptSkillDescriptionRune+10)) {
		t.Fatalf("expected long descriptions to be trimmed, got %q", got)
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

func assertNoTemplateMarkers(t *testing.T, prompt string) {
	t.Helper()
	markers := []string{
		"{{ACTIVE_SKILL_BLOCK}}",
	}
	for _, marker := range markers {
		if strings.Contains(prompt, marker) {
			t.Fatalf("expected template marker %q to be rendered, got %q", marker, prompt)
		}
	}
}
