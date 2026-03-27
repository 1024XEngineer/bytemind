package agent

import (
	_ "embed"
	"fmt"
	"strings"

	"bytemind/internal/skills"
)

//go:embed prompts/system.md
var systemPromptSource string

func systemPrompt(workspace, approvalPolicy string, skill *skills.Skill) string {
	replacer := strings.NewReplacer(
		"{{WORKSPACE}}", workspace,
		"{{APPROVAL_POLICY}}", approvalPolicy,
	)
	prompt := replacer.Replace(systemPromptSource)
	if skill == nil {
		return prompt
	}

	var builder strings.Builder
	builder.WriteString(prompt)
	builder.WriteString("\n\nActive project skill:\n")
	builder.WriteString(fmt.Sprintf("- Name: /%s\n", skill.Name))
	if desc := skill.DisplayDescription(); desc != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", desc))
	}
	builder.WriteString("- Follow these additional instructions when they help with the user's request.\n")
	if body := skill.Instructions(); body != "" {
		builder.WriteString("\n")
		builder.WriteString(body)
	}
	return builder.String()
}
