package prompts

import (
	_ "embed"
	"strings"
	"time"
)

const (
	ModeBuild = "build"
	ModePlan  = "plan"
)

//go:embed prompt/main.md
var mainPromptSource string

//go:embed prompt/mode/build.md
var buildModePromptSource string

//go:embed prompt/mode/plan.md
var planModePromptSource string

type Skill struct {
	Name        string
	Description string
	Enabled     bool
}

type Input struct {
	Workspace      string
	ApprovalPolicy string
	ProviderType   string
	Model          string
	MaxIterations  int
	Mode           string
	Platform       string
	Now            time.Time
	Skills         []Skill
	Tools          []string
}

func Compose(input Input) string {
	input.Mode = NormalizeMode(input.Mode)
	parts := []string{
		strings.TrimSpace(mainPromptSource),
		strings.TrimSpace(modePromptSource(input.Mode)),
		renderSystemBlock(input),
		renderInstructionBlock(input.Workspace),
	}
	return strings.Join(filterParts(parts), "\n\n")
}

func NormalizeMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ModePlan:
		return ModePlan
	default:
		return ModeBuild
	}
}

func modePromptSource(mode string) string {
	if NormalizeMode(mode) == ModePlan {
		return planModePromptSource
	}
	return buildModePromptSource
}

func filterParts(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}
