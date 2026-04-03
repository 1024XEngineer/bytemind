package agent

import (
	"time"

	promptpkg "bytemind/internal/agent/prompts"
)

type PromptSkill = promptpkg.Skill

type PromptInput struct {
	Workspace      string
	ApprovalPolicy string
	ProviderType   string
	Model          string
	MaxIterations  int
	Mode           string
	Platform       string
	Now            time.Time
	Skills         []PromptSkill
	Tools          []string
}

func systemPrompt(input PromptInput) string {
	return promptpkg.Compose(promptpkg.Input{
		Workspace:      input.Workspace,
		ApprovalPolicy: input.ApprovalPolicy,
		ProviderType:   input.ProviderType,
		Model:          input.Model,
		MaxIterations:  input.MaxIterations,
		Mode:           input.Mode,
		Platform:       input.Platform,
		Now:            input.Now,
		Skills:         input.Skills,
		Tools:          input.Tools,
	})
}
