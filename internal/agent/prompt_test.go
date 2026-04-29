package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func TestSystemPromptRendersMainModeSystemAndInstruction(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("Use rg for search before broad shell scans."), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := systemPrompt(PromptInput{
		Workspace:                    workspace,
		ApprovalPolicy:               "on-request",
		ApprovalMode:                 "away",
		AwayPolicy:                   "fail_fast",
		SandboxEnabled:               true,
		SystemSandbox:                "best_effort",
		SystemSandboxBackend:         "linux_unshare",
		SystemSandboxRequiredCapable: true,
		SystemSandboxShellNetwork:    true,
		SystemSandboxWorkerNetwork:   false,
		SystemSandboxFallback:        false,
		SystemSandboxStatus:          `system sandbox backend "linux_unshare" is active`,
		Model:                        "gpt-5.4-mini",
		Mode:                         "plan",
		Platform:                     "linux/amd64",
		Now:                          time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
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
	assertContains(t, prompt, "Your capabilities:")
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "plan")
	assertContains(t, prompt, "[Runtime Context]")
	assertContains(t, prompt, "workspace_root: "+workspace)
	assertContains(t, prompt, "platform: linux/amd64")
	assertContains(t, prompt, "date: 2026-04-03")
	assertContains(t, prompt, "model: gpt-5.4-mini")
	assertContains(t, prompt, "mode: plan")
	assertContains(t, prompt, "approval_policy: on-request")
	assertContains(t, prompt, "approval_mode: away")
	assertContains(t, prompt, "away_policy: fail_fast")
	assertContains(t, prompt, "sandbox_enabled: true")
	assertContains(t, prompt, "system_sandbox_mode: best_effort")
	assertContains(t, prompt, "system_sandbox_backend: linux_unshare")
	assertContains(t, prompt, "system_sandbox_required_capable: true")
	assertContains(t, prompt, "system_sandbox_capability_level: none")
	assertContains(t, prompt, "system_sandbox_shell_network_isolation: true")
	assertContains(t, prompt, "system_sandbox_worker_network_isolation: false")
	assertContains(t, prompt, "system_sandbox_fallback: false")
	assertContains(t, prompt, `system_sandbox_status: system sandbox backend "linux_unshare" is active`)
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "Skills are reusable task profiles available in this session")
	assertContains(t, prompt, "- review: Review code changes for regressions.")
	assertContains(t, prompt, "[Available Tools]")
	assertContains(t, prompt, "- list_files")
	assertContains(t, prompt, "- read_file")
	assertContains(t, prompt, "[Active Skill]")
	assertContains(t, prompt, "Use this skill when it is relevant to the user's request.")
	assertContains(t, prompt, "Follow the workflow and output contract defined here.")
	assertContains(t, prompt, "Tool Policy: allowlist")
	assertContains(t, prompt, "[Instructions]")
	assertContains(t, prompt, "Instructions from:")
	assertContains(t, prompt, "Use rg for search before broad shell scans.")
	assertNotContains(t, prompt, "The contents of the AGENTS.md file at the root of the repo and any directories from the CWD up to the root are included with the developer message")
	assertNotContains(t, prompt, "Primary objective:")
	assertNotContains(t, prompt, "Tool Guidelines")
	assertNotContains(t, prompt, "Only the active skill block is currently in effect.")
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
	assertContains(t, prompt, "approval_mode: interactive")
	assertContains(t, prompt, "away_policy: auto_deny_continue")
	assertContains(t, prompt, "sandbox_enabled: false")
	assertContains(t, prompt, "system_sandbox_mode: off")
	assertContains(t, prompt, "system_sandbox_backend: none")
	assertContains(t, prompt, "system_sandbox_required_capable: false")
	assertContains(t, prompt, "system_sandbox_capability_level: none")
	assertContains(t, prompt, "system_sandbox_shell_network_isolation: false")
	assertContains(t, prompt, "system_sandbox_worker_network_isolation: false")
	assertContains(t, prompt, "system_sandbox_fallback: false")
	if strings.Contains(prompt, "[Instructions]") {
		t.Fatalf("did not expect instruction block in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "\n[Active Skill]\n") {
		t.Fatalf("did not expect active skill block in prompt: %q", prompt)
	}
	assertNoTemplateMarkers(t, prompt)
}

func TestSystemPromptIncludesCurrentPlanStateWhenPresent(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "never",
		Model:          "deepseek-chat",
		Mode:           "build",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Plan: planpkg.State{
			Goal:                "Harden plan mode",
			Phase:               planpkg.PhaseConvergeReady,
			Steps:               []planpkg.Step{{Title: "Rewrite plan prompt", Status: planpkg.StepPending}},
			DecisionGaps:        []string{"Pick the execution trigger copy"},
			ScopeDefined:        true,
			RiskRollbackDefined: true,
		},
	})

	assertContains(t, prompt, "[Current Plan State]")
	assertContains(t, prompt, "phase: converge_ready")
	assertContains(t, prompt, "goal: Harden plan mode")
	assertContains(t, prompt, "decision_gaps: Pick the execution trigger copy")
	assertContains(t, prompt, "steps:")
	assertContains(t, prompt, "execution_readiness: scope=yes, risks_rollback=yes, verification=no")
}

func TestSystemPromptIncludesActiveChoiceWhenPresent(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "never",
		Model:          "deepseek-chat",
		Mode:           "plan",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Plan: planpkg.State{
			Phase:        planpkg.PhaseClarify,
			Steps:        []planpkg.Step{{Title: "Choose frontend", Status: planpkg.StepPending}},
			DecisionGaps: []string{"Choose the frontend stack"},
			ActiveChoice: &planpkg.ActiveChoice{
				ID:       "frontend_stack",
				Kind:     "clarify",
				Question: "前端希望走哪条路线？",
				Options: []planpkg.ChoiceOption{
					{ID: "a", Shortcut: "A", Title: "FastAPI + Jinja2", Recommended: true},
					{ID: "b", Shortcut: "B", Title: "Flask + Jinja2"},
				},
			},
		},
	})

	assertContains(t, prompt, "active_choice:")
	assertContains(t, prompt, "- question: 前端希望走哪条路线？")
	assertContains(t, prompt, "- [A] FastAPI + Jinja2 -- recommended")
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

func TestFormatSkillsKeepsSkillDescriptionsAsIs(t *testing.T) {
	got := formatSkills([]PromptSkill{
		{Name: "review", Description: "Review with strict correctness focus.", Enabled: true},
	})

	assertContains(t, got, "- review: Review with strict correctness focus.")
}

func TestRenderActiveSkillPromptKeepsSkillFieldsAsIs(t *testing.T) {
	out := renderActiveSkillPrompt(&PromptActiveSkill{
		Name:         "review",
		Description:  "Review code changes with a strict correctness focus.",
		WhenToUse:    "Use when the user asks for a code review.",
		Instructions: "Prioritize regression risk and missing tests.",
		Args: map[string]string{
			"base_ref": "main",
		},
		ToolPolicy: "allowlist",
		Tools:      []string{"read_file"},
	})

	assertContains(t, out, "Description: Review code changes with a strict correctness focus.")
	assertContains(t, out, "When To Use: Use when the user asks for a code review.")
	assertContains(t, out, "- base_ref=main")
	assertContains(t, out, "Prioritize regression risk and missing tests.")
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

func assertNotContains(t *testing.T, prompt, needle string) {
	t.Helper()
	if strings.Contains(prompt, needle) {
		t.Fatalf("did not expect %q in prompt, got %q", needle, prompt)
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
