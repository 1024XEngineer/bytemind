package agent

import (
	"strings"
	"testing"
	"time"
)

func TestSystemPromptRendersMainModeSystemAndInstruction(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "on-request",
		Model:          "gpt-5.4-mini",
		Mode:           "build",
		Platform:       "linux/amd64",
		Now:            time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		Skills: []PromptSkill{
			{Name: "review", Description: "Review code changes for bugs and regressions.", Enabled: true},
		},
		Tools:       []string{"read_file", "search_text", "list_files"},
		Instruction: "Instructions from: /tmp/workspace/AGENTS.md\n- Keep diffs minimal.",
	})

	assertContains(t, prompt, "You are OpenCode")
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "build")
	assertContains(t, prompt, "workspace_root: /tmp/workspace")
	assertContains(t, prompt, "platform: linux/amd64")
	assertContains(t, prompt, "date: 2026-04-02")
	assertContains(t, prompt, "model: gpt-5.4-mini")
	assertContains(t, prompt, "approval_policy: on-request")
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "review: Review code changes for bugs and regressions.")
	assertContains(t, prompt, "[Available Tools]")
	assertContains(t, prompt, "- list_files")
	assertContains(t, prompt, "[Instructions]")
	assertContains(t, prompt, "AGENTS.md")
}

func TestSystemPromptOmitsInstructionWhenEmpty(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "never",
		Model:          "deepseek-coder",
		Mode:           "plan",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	})

	assertContains(t, prompt, "Plan mode ACTIVE")
	assertContains(t, prompt, "[Runtime Context]")
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "- none")
	assertContains(t, prompt, "[Available Tools]")
	if strings.Contains(prompt, "[Instructions]") {
		t.Fatalf("did not expect instructions block in prompt: %q", prompt)
	}
}

func TestTaskPromptLookup(t *testing.T) {
	for _, task := range []string{"explore", "compaction", "summary", "title"} {
		if strings.TrimSpace(taskPrompt(task)) == "" {
			t.Fatalf("expected non-empty task prompt for %q", task)
		}
	}
	if taskPrompt("unknown-task") != "" {
		t.Fatalf("expected empty prompt for unknown task")
	}
}

func assertContains(t *testing.T, prompt, needle string) {
	t.Helper()
	if !strings.Contains(prompt, needle) {
		t.Fatalf("expected %q in prompt, got %q", needle, prompt)
	}
}
