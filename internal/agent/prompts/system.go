package prompts

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

func renderSystemBlock(input Input) string {
	contextLines := []string{
		"[Runtime Context]",
		"workspace_root: " + strings.TrimSpace(input.Workspace),
		"cwd: " + strings.TrimSpace(input.Workspace),
		"platform: " + defaultPlatform(input.Platform),
		"date: " + defaultDate(input.Now),
		"provider_type: " + defaultProviderType(input.ProviderType),
		"model: " + defaultModel(input.Model),
		"approval_policy: " + strings.TrimSpace(input.ApprovalPolicy),
		"mode: " + NormalizeMode(input.Mode),
		"max_iterations: " + fmt.Sprintf("%d", input.MaxIterations),
	}

	skills := renderSkillsBlock(input.Skills)
	tools := renderToolsBlock(input.Tools)
	return strings.Join([]string{
		strings.Join(contextLines, "\n"),
		skills,
		tools,
	}, "\n\n")
}

func renderSkillsBlock(skills []Skill) string {
	lines := []string{"[Skills]"}
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if name == "" || description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s enabled=%t", name, description, skill.Enabled))
	}
	if len(lines) == 1 {
		lines = append(lines, "- (none)")
	}
	return strings.Join(lines, "\n")
}

func renderToolsBlock(toolNames []string) string {
	normalized := normalizeToolNames(toolNames)
	lines := []string{"[Tools]"}
	if len(normalized) == 0 {
		lines = append(lines, "- (none)")
		return strings.Join(lines, "\n")
	}
	for _, name := range normalized {
		lines = append(lines, "- "+name)
	}
	return strings.Join(lines, "\n")
}

func normalizeToolNames(toolNames []string) []string {
	if len(toolNames) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(toolNames))
	normalized := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	sort.Strings(normalized)
	return normalized
}

func defaultPlatform(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

func defaultDate(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return now.Format("2006-01-02")
}

func defaultProviderType(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return "openai-compatible"
}

func defaultModel(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return "unknown"
}
