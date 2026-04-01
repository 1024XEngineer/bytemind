package agent

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"bytemind/internal/session"
)

//go:embed prompts/core.md
var corePromptSource string

//go:embed prompts/mode-build.md
var buildModePromptSource string

//go:embed prompts/mode-plan.md
var planModePromptSource string

//go:embed prompts/block-environment.md
var environmentPromptSource string

//go:embed prompts/block-session.md
var sessionPromptSource string

//go:embed prompts/block-plan.md
var planPromptSource string

//go:embed prompts/block-repo-rules.md
var repoRulesPromptSource string

//go:embed prompts/block-skills-summary.md
var skillsPromptSource string

//go:embed prompts/block-output-contract.md
var outputContractPromptSource string

//go:embed prompts/provider-openai.md
var openAIProviderPromptSource string

//go:embed prompts/provider-anthropic.md
var anthropicProviderPromptSource string

//go:embed prompts/provider-gemini.md
var geminiProviderPromptSource string

type PromptSkill struct {
	Name        string
	Description string
	Enabled     bool
}

type PromptInput struct {
	Workspace        string
	ApprovalPolicy   string
	ProviderType     string
	Model            string
	MaxIterations    int
	Mode             string
	Platform         string
	Now              time.Time
	SessionSummary   string
	Plan             []session.PlanItem
	RepoRulesSummary string
	Skills           []PromptSkill
	OutputContract   string
}

func systemPrompt(input PromptInput) string {
	parts := []string{
		strings.TrimSpace(corePromptSource),
		strings.TrimSpace(modePrompt(input.Mode)),
		renderEnvironmentPrompt(input),
		renderProviderPrompt(input.ProviderType, input.Model),
		renderSessionPrompt(input.SessionSummary),
		renderPlanPrompt(input.Plan),
		renderRepoRulesPrompt(input.RepoRulesSummary),
		renderSkillsPrompt(input.Skills),
		renderOutputContractPrompt(input.OutputContract),
	}
	return strings.Join(filterPromptParts(parts), "\n\n")
}

func modePrompt(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan":
		return planModePromptSource
	default:
		return buildModePromptSource
	}
}

func renderEnvironmentPrompt(input PromptInput) string {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = runtime.GOOS + "/" + runtime.GOARCH
	}

	providerType := strings.TrimSpace(input.ProviderType)
	if providerType == "" {
		providerType = "openai-compatible"
	}

	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "build"
	}

	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "unknown"
	}

	replacer := strings.NewReplacer(
		"{{CWD}}", input.Workspace,
		"{{WORKSPACE}}", input.Workspace,
		"{{PLATFORM}}", platform,
		"{{DATE}}", now.Format("2006-01-02"),
		"{{APPROVAL_POLICY}}", input.ApprovalPolicy,
		"{{MODE}}", mode,
		"{{PROVIDER_TYPE}}", providerType,
		"{{MODEL}}", model,
		"{{MAX_ITERATIONS}}", fmt.Sprintf("%d", input.MaxIterations),
	)
	return replacer.Replace(strings.TrimSpace(environmentPromptSource))
}

func renderPlanPrompt(plan []session.PlanItem) string {
	if len(plan) == 0 {
		return ""
	}

	lines := make([]string, 0, len(plan))
	for _, item := range plan {
		step := strings.TrimSpace(item.Step)
		if step == "" {
			continue
		}
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "pending"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", status, step))
	}
	if len(lines) == 0 {
		return ""
	}

	return strings.ReplaceAll(strings.TrimSpace(planPromptSource), "{{PLAN_ITEMS}}", strings.Join(lines, "\n"))
}

func renderSessionPrompt(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(sessionPromptSource), "{{SESSION_SUMMARY}}", summary)
}

func renderRepoRulesPrompt(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(repoRulesPromptSource), "{{REPO_RULES_SUMMARY}}", summary)
}

func renderSkillsPrompt(skills []PromptSkill) string {
	if len(skills) == 0 {
		return ""
	}

	lines := make([]string, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if name == "" || description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s enabled=%t", name, description, skill.Enabled))
	}
	if len(lines) == 0 {
		return ""
	}

	return strings.ReplaceAll(strings.TrimSpace(skillsPromptSource), "{{SKILLS_SUMMARY}}", strings.Join(lines, "\n"))
}

func renderOutputContractPrompt(contract string) string {
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(outputContractPromptSource), "{{OUTPUT_CONTRACT}}", contract)
}

func renderProviderPrompt(providerType, model string) string {
	switch promptModelFamily(providerType, model) {
	case "anthropic":
		return strings.TrimSpace(anthropicProviderPromptSource)
	case "gemini":
		return strings.TrimSpace(geminiProviderPromptSource)
	case "openai":
		return strings.TrimSpace(openAIProviderPromptSource)
	default:
		return ""
	}
}

func promptModelFamily(providerType, model string) string {
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	model = strings.ToLower(strings.TrimSpace(model))

	switch {
	case providerType == "anthropic", strings.Contains(model, "claude"):
		return "anthropic"
	case providerType == "gemini", strings.Contains(model, "gemini"):
		return "gemini"
	case providerType == "openai", providerType == "openai-compatible":
		return "openai"
	case strings.Contains(model, "gpt"), strings.Contains(model, "codex"):
		return "openai"
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "openai"
	default:
		return ""
	}
}

func filterPromptParts(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}
