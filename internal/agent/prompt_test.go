package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSystemPromptRendersModeSystemAndAgentsInstructions(t *testing.T) {
	workspace := t.TempDir()
	agents := "- Follow project conventions."
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := systemPrompt(PromptInput{
		Workspace:      workspace,
		ApprovalPolicy: "on-request",
		ProviderType:   "openai-compatible",
		Model:          "gpt-5.4-mini",
		MaxIterations:  32,
		Mode:           "build",
		Platform:       "linux/amd64",
		Now:            time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Skills: []PromptSkill{
			{Name: "review", Description: "Review for regressions.", Enabled: true},
		},
		Tools: []string{"write_file", "read_file"},
	})

	assertContains(t, prompt, "You are ByteMind")
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "build")
	assertContains(t, prompt, "[Runtime Context]")
	assertContains(t, prompt, workspace)
	assertContains(t, prompt, "provider_type: openai-compatible")
	assertContains(t, prompt, "model: gpt-5.4-mini")
	assertContains(t, prompt, "approval_policy: on-request")
	assertContains(t, prompt, "max_iterations: 32")
	assertContains(t, prompt, "[Skills]")
	assertContains(t, prompt, "- review: Review for regressions. enabled=true")
	assertContains(t, prompt, "[Tools]")
	assertContains(t, prompt, "- read_file")
	assertContains(t, prompt, "- write_file")
	assertContains(t, prompt, "[Instruction Boundary]")
	assertContains(t, prompt, "The main system prompt defines global behavior.")
	assertContains(t, prompt, "[Repository Instructions]")
	assertContains(t, prompt, "source: AGENTS.md")
	assertContains(t, prompt, agents)
}

func TestSystemPromptDefaultsToBuildAndSkipsMissingAgentsInstructions(t *testing.T) {
	workspace := t.TempDir()
	prompt := systemPrompt(PromptInput{
		Workspace:      workspace,
		ApprovalPolicy: "never",
		ProviderType:   "anthropic",
		Model:          "claude-sonnet-4",
		MaxIterations:  16,
		Mode:           "",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})

	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "build")
	assertContains(t, prompt, "[Skills]\n- (none)")
	assertContains(t, prompt, "[Tools]\n- (none)")
	if strings.Contains(prompt, "Required final answer structure:") {
		t.Fatalf("did not expect plan mode block in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "[Repository Instructions]") {
		t.Fatalf("did not expect AGENTS block when AGENTS.md is missing: %q", prompt)
	}
}

func TestSystemPromptUsesPlanModePromptWhenRequested(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      t.TempDir(),
		ApprovalPolicy: "never",
		Mode:           "plan",
		Now:            time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})

	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "plan")
	assertContains(t, prompt, "Required final answer structure:")
	assertContains(t, prompt, "Next Action")
}

func assertContains(t *testing.T, prompt, needle string) {
	t.Helper()
	if !strings.Contains(prompt, needle) {
		t.Fatalf("expected %q in prompt, got %q", needle, prompt)
	}
}
