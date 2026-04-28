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

	planpkg "bytemind/internal/plan"
)

const (
	maxPromptSkillEntries         = 12
	maxPromptSkillDescriptionRune = 140
)

//go:embed prompts/default.md
var mainPromptSource string

//go:embed prompts/mode/build.md
var buildModePromptSource string

//go:embed prompts/mode/plan.md
var planModePromptSource string

//go:embed prompts/block-active-skill.md
var activeSkillPromptSource string

type PromptSkill struct {
	Name        string
	Description string
	Enabled     bool
}

type PromptActiveSkill struct {
	Name         string
	Description  string
	WhenToUse    string
	Instructions string
	Args         map[string]string
	ToolPolicy   string
	Tools        []string
}

type PromptInput struct {
	Workspace                    string
	ApprovalPolicy               string
	ApprovalMode                 string
	AwayPolicy                   string
	SandboxEnabled               bool
	SystemSandbox                string
	SystemSandboxBackend         string
	SystemSandboxRequiredCapable bool
	SystemSandboxCapabilityLevel string
	SystemSandboxShellNetwork    bool
	SystemSandboxWorkerNetwork   bool
	SystemSandboxFallback        bool
	SystemSandboxStatus          string
	Model                        string
	Mode                         string
	Platform                     string
	Now                          time.Time
	Skills                       []PromptSkill
	Tools                        []string
	Plan                         planpkg.State
	ActiveSkill                  *PromptActiveSkill
	Instruction                  string
}

func systemPrompt(input PromptInput) string {
	parts := []string{
		strings.TrimSpace(mainPromptSource),
		strings.TrimSpace(modePrompt(input.Mode)),
		renderSystemBlock(input),
		renderActiveSkillPrompt(input.ActiveSkill),
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

func loadAGENTSInstruction(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		promptDebugf("AGENTS skip: empty workspace")
		return ""
	}
	path := filepath.Join(workspace, "AGENTS.md")
	content, err := os.ReadFile(path)
	if err != nil {
		promptDebugf("AGENTS skip: failed to read %s: %v", path, err)
		return ""
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		promptDebugf("AGENTS skip: file is empty: %s", path)
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	promptDebugf("AGENTS loaded: %s", path)
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
		approval = "on-request"
	}
	approvalMode := strings.TrimSpace(input.ApprovalMode)
	if approvalMode == "" {
		approvalMode = "interactive"
	}
	awayPolicy := strings.TrimSpace(input.AwayPolicy)
	if awayPolicy == "" {
		awayPolicy = "auto_deny_continue"
	}
	systemSandbox := strings.TrimSpace(input.SystemSandbox)
	if systemSandbox == "" {
		systemSandbox = "off"
	}
	systemSandboxBackend := strings.TrimSpace(input.SystemSandboxBackend)
	if systemSandboxBackend == "" {
		systemSandboxBackend = "none"
	}
	systemSandboxFallback := "false"
	if input.SystemSandboxFallback {
		systemSandboxFallback = "true"
	}
	systemSandboxRequiredCapable := "false"
	if input.SystemSandboxRequiredCapable {
		systemSandboxRequiredCapable = "true"
	}
	systemSandboxCapabilityLevel := strings.ToLower(strings.TrimSpace(input.SystemSandboxCapabilityLevel))
	if systemSandboxCapabilityLevel == "" {
		systemSandboxCapabilityLevel = "none"
	}
	systemSandboxShellNetwork := "false"
	if input.SystemSandboxShellNetwork {
		systemSandboxShellNetwork = "true"
	}
	systemSandboxWorkerNetwork := "false"
	if input.SystemSandboxWorkerNetwork {
		systemSandboxWorkerNetwork = "true"
	}
	systemSandboxStatus := strings.TrimSpace(input.SystemSandboxStatus)
	sandboxEnabled := "false"
	if input.SandboxEnabled {
		sandboxEnabled = "true"
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
		fmt.Sprintf("approval_mode: %s", approvalMode),
		fmt.Sprintf("away_policy: %s", awayPolicy),
		fmt.Sprintf("sandbox_enabled: %s", sandboxEnabled),
		fmt.Sprintf("system_sandbox_mode: %s", systemSandbox),
		fmt.Sprintf("system_sandbox_backend: %s", systemSandboxBackend),
		fmt.Sprintf("system_sandbox_required_capable: %s", systemSandboxRequiredCapable),
		fmt.Sprintf("system_sandbox_capability_level: %s", systemSandboxCapabilityLevel),
		fmt.Sprintf("system_sandbox_shell_network_isolation: %s", systemSandboxShellNetwork),
		fmt.Sprintf("system_sandbox_worker_network_isolation: %s", systemSandboxWorkerNetwork),
		fmt.Sprintf("system_sandbox_fallback: %s", systemSandboxFallback),
		"",
		"[Available Skills]",
		"- Skills are reusable task profiles available in this session. Only the [Active Skill] block, when present, is currently in effect.",
		formatSkills(input.Skills),
		"",
		"[Available Tools]",
		formatTools(input.Tools),
	}
	if planState := strings.TrimSpace(planpkg.RenderPromptStateBlock(input.Plan)); planState != "" {
		lines = append(lines, "", "[Current Plan State]", planState)
	}
	if systemSandboxStatus != "" {
		lines = append(lines, "", fmt.Sprintf("system_sandbox_status: %s", systemSandboxStatus))
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
		if name == "" {
			continue
		}
		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = "No description provided."
		}
		description = trimPromptText(description, maxPromptSkillDescriptionRune)
		lines = append(lines, fmt.Sprintf("- %s: %s", name, description))
	}
	if len(lines) == 0 {
		return "- none"
	}
	sort.Strings(lines)
	if len(lines) > maxPromptSkillEntries {
		remaining := len(lines) - maxPromptSkillEntries
		lines = append(lines[:maxPromptSkillEntries], fmt.Sprintf("- ... and %d more skill(s)", remaining))
	}
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

func renderActiveSkillPrompt(skill *PromptActiveSkill) string {
	if skill == nil {
		return ""
	}

	name := strings.TrimSpace(skill.Name)
	description := strings.TrimSpace(skill.Description)
	whenToUse := strings.TrimSpace(skill.WhenToUse)
	instructions := strings.TrimSpace(skill.Instructions)
	toolPolicy := strings.TrimSpace(skill.ToolPolicy)
	if name == "" && description == "" && whenToUse == "" && instructions == "" && toolPolicy == "" {
		return ""
	}

	lines := make([]string, 0, 16)
	if name != "" {
		lines = append(lines, "Name: "+name)
	}
	if description != "" {
		lines = append(lines, "Description: "+description)
	}
	if whenToUse != "" {
		lines = append(lines, "When To Use: "+whenToUse)
	}
	if len(skill.Args) > 0 {
		keys := make([]string, 0, len(skill.Args))
		for key := range skill.Args {
			if strings.TrimSpace(key) != "" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			lines = append(lines, "Args:")
			for _, key := range keys {
				value := strings.TrimSpace(skill.Args[key])
				if value == "" {
					continue
				}
				lines = append(lines, fmt.Sprintf("- %s=%s", key, value))
			}
		}
	}
	if toolPolicy != "" {
		lines = append(lines, "Tool Policy: "+toolPolicy)
	}
	if len(skill.Tools) > 0 {
		tools := make([]string, 0, len(skill.Tools))
		for _, tool := range skill.Tools {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
		if len(tools) > 0 {
			sort.Strings(tools)
			lines = append(lines, "Tool Items: "+strings.Join(tools, ", "))
		}
	}
	if instructions != "" {
		lines = append(lines, "", "Instructions:", instructions)
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.ReplaceAll(strings.TrimSpace(activeSkillPromptSource), "{{ACTIVE_SKILL_BLOCK}}", strings.Join(lines, "\n"))
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

func trimPromptText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func promptDebugEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("BYTEMIND_DEBUG_PROMPT")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func promptDebugf(format string, args ...any) {
	if !promptDebugEnabled() {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[bytemind][prompt] "+format+"\n", args...)
}
