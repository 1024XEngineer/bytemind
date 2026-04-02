package agent

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

//go:embed prompts/main.md
var mainPromptSource string

//go:embed prompts/mode/build.md
var buildModePromptSource string

//go:embed prompts/mode/plan.md
var planModePromptSource string

//go:embed prompts/max-steps.txt
var maxStepsPromptSource string

//go:embed prompts/task/explore.txt
var exploreTaskPromptSource string

//go:embed prompts/task/compaction.txt
var compactionTaskPromptSource string

//go:embed prompts/task/summary.txt
var summaryTaskPromptSource string

//go:embed prompts/task/title.txt
var titleTaskPromptSource string

type PromptSkill struct {
	Name        string
	Description string
	Enabled     bool
}

type PromptInput struct {
	Workspace      string
	ApprovalPolicy string
	Model          string
	Mode           string
	Platform       string
	Now            time.Time
	Skills         []PromptSkill
	Tools          []string
	Instruction    string
}

func systemPrompt(input PromptInput) string {
	parts := []string{
		strings.TrimSpace(mainPromptSource),
		strings.TrimSpace(modePrompt(input.Mode)),
		renderSystemBlock(input),
		renderInstructionBlock(input.Instruction),
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

func maxStepsPrompt() string {
	return strings.TrimSpace(maxStepsPromptSource)
}

func taskPrompt(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "explore":
		return strings.TrimSpace(exploreTaskPromptSource)
	case "compaction":
		return strings.TrimSpace(compactionTaskPromptSource)
	case "summary":
		return strings.TrimSpace(summaryTaskPromptSource)
	case "title":
		return strings.TrimSpace(titleTaskPromptSource)
	default:
		return ""
	}
}

func loadAGENTSInstruction(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	path := filepath.Join(workspace, "AGENTS.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	return "Instructions from: " + path + "\n" + text
}

func renderSystemBlock(input PromptInput) string {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = runtime.GOOS + "/" + runtime.GOARCH
	}

	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "build"
	}

	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "unknown"
	}

	workspace := strings.TrimSpace(input.Workspace)
	if workspace == "" {
		workspace = "."
	}
	approval := strings.TrimSpace(input.ApprovalPolicy)
	if approval == "" {
		approval = "never"
	}
	gitRepo := "no"
	if isGitRepository(workspace) {
		gitRepo = "yes"
	}

	lines := []string{
		"[Runtime Context]",
		fmt.Sprintf("workspace_root: %s", workspace),
		fmt.Sprintf("cwd: %s", workspace),
		fmt.Sprintf("platform: %s", platform),
		fmt.Sprintf("date: %s", now.Format("2006-01-02")),
		fmt.Sprintf("is_git_repo: %s", gitRepo),
		fmt.Sprintf("model: %s", model),
		fmt.Sprintf("mode: %s", mode),
		fmt.Sprintf("approval_policy: %s", approval),
		"",
		"[Available Skills]",
		formatSkills(input.Skills),
		"",
		"[Available Tools]",
		formatTools(input.Tools),
	}
	return strings.Join(lines, "\n")
}

func formatSkills(skills []PromptSkill) string {
	if len(skills) == 0 {
		return "- none"
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
		return "- none"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func formatTools(tools []string) string {
	if len(tools) == 0 {
		return "- none"
	}
	seen := make(map[string]struct{}, len(tools))
	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		lines = append(lines, "- "+name)
	}
	if len(lines) == 0 {
		return "- none"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func renderInstructionBlock(instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return ""
	}
	return "[Instructions]\n" + instruction
}

func isGitRepository(workspace string) bool {
	if strings.TrimSpace(workspace) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(workspace, ".git"))
	return err == nil
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
