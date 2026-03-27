package agent

import (
	"strings"
	"testing"
)

func TestSystemPromptRendersTemplateVariables(t *testing.T) {
	workspace := "/tmp/workspace"
	approvalPolicy := "on-request"

	prompt := systemPrompt(workspace, approvalPolicy)

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
