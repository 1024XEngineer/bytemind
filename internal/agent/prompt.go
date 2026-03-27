package agent

import (
	_ "embed"
	"strings"
)

//go:embed prompts/system.md
var systemPromptSource string

func systemPrompt(workspace, approvalPolicy string) string {
	replacer := strings.NewReplacer(
		"{{WORKSPACE}}", workspace,
		"{{APPROVAL_POLICY}}", approvalPolicy,
	)
	return replacer.Replace(systemPromptSource)
}
