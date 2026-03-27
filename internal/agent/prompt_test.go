package agent

import (
	"strings"
	"testing"

	"bytemind/internal/skills"
)

func TestSystemPromptRendersTemplateVariables(t *testing.T) {
	workspace := "/tmp/workspace"
	approvalPolicy := "on-request"

	prompt := systemPrompt(workspace, approvalPolicy, nil)

	if !strings.Contains(prompt, workspace) {
		t.Fatalf("expected workspace %q in prompt, got %q", workspace, prompt)
	}
	if !strings.Contains(prompt, approvalPolicy) {
		t.Fatalf("expected approval policy %q in prompt, got %q", approvalPolicy, prompt)
	}
	if strings.Contains(prompt, "{{WORKSPACE}}") || strings.Contains(prompt, "{{APPROVAL_POLICY}}") {
		t.Fatalf("expected template variables to be rendered, got %q", prompt)
	}
}

func TestSystemPromptIncludesActiveSkillInstructions(t *testing.T) {
	prompt := systemPrompt("/tmp/workspace", "on-request", &skills.Skill{
		Name:        "review",
		Description: "Review changes carefully",
		Content:     "---\nname: review\n---\nPrefer risk-first feedback.",
	})

	if !strings.Contains(prompt, "Active project skill") {
		t.Fatalf("expected active skill header in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "/review") {
		t.Fatalf("expected skill name in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Prefer risk-first feedback.") {
		t.Fatalf("expected skill instructions in prompt, got %q", prompt)
	}
}
