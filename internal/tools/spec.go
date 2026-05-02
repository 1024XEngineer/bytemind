package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

type SafetyClass string

const (
	SafetyClassSafe        SafetyClass = "safe"
	SafetyClassModerate    SafetyClass = "moderate"
	SafetyClassSensitive   SafetyClass = "sensitive"
	SafetyClassDestructive SafetyClass = "destructive"
)

type ToolSpec struct {
	Name            string
	ReadOnly        bool
	ConcurrencySafe bool
	Destructive     bool
	SafetyClass     SafetyClass
	StrictArgs      bool
	SearchHint      string
	AllowedModes    []planpkg.AgentMode
	DefaultTimeoutS int
	MaxTimeoutS     int
	MaxResultChars  int
}

type ToolSpecProvider interface {
	Spec() ToolSpec
}

func MergeToolSpec(base, override ToolSpec) ToolSpec {
	if strings.TrimSpace(override.Name) != "" {
		base.Name = override.Name
	}
	if override.ReadOnly {
		base.ReadOnly = true
	}
	if override.Destructive {
		base.Destructive = true
	}
	if override.SafetyClass != "" {
		base.SafetyClass = override.SafetyClass
	}
	if strings.TrimSpace(override.SearchHint) != "" {
		base.SearchHint = override.SearchHint
	}
	if len(override.AllowedModes) > 0 {
		base.AllowedModes = override.AllowedModes
	}
	if override.DefaultTimeoutS > 0 {
		base.DefaultTimeoutS = override.DefaultTimeoutS
	}
	if override.MaxTimeoutS > 0 {
		base.MaxTimeoutS = override.MaxTimeoutS
	}
	if override.MaxResultChars > 0 {
		base.MaxResultChars = override.MaxResultChars
	}
	return NormalizeToolSpec(base)
}

func DefaultToolSpec(def llm.ToolDefinition) ToolSpec {
	name := strings.TrimSpace(def.Function.Name)
	spec := ToolSpec{
		Name:            name,
		ConcurrencySafe: true,
		StrictArgs:      true,
		AllowedModes:    defaultAllowedModes(name),
		DefaultTimeoutS: 30,
		MaxTimeoutS:     300,
		MaxResultChars:  64 * 1024,
	}

	switch name {
	case "list_files", "read_file", "search_text", "web_search", "web_fetch":
		spec.ReadOnly = true
		spec.SafetyClass = SafetyClassSafe
	case "update_plan":
		spec.SafetyClass = SafetyClassModerate
		spec.DefaultTimeoutS = 10
		spec.MaxTimeoutS = 10
		spec.MaxResultChars = 32 * 1024
	case "run_shell":
		spec.SafetyClass = SafetyClassSensitive
	case "write_file", "replace_in_file", "apply_patch":
		spec.ConcurrencySafe = false
		spec.Destructive = true
		spec.SafetyClass = SafetyClassDestructive
	default:
		spec.SafetyClass = SafetyClassModerate
	}

	return NormalizeToolSpec(spec)
}

func NormalizeToolSpec(spec ToolSpec) ToolSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.SearchHint = strings.TrimSpace(spec.SearchHint)
	if spec.SafetyClass == "" {
		switch {
		case spec.Destructive:
			spec.SafetyClass = SafetyClassDestructive
		case spec.ReadOnly:
			spec.SafetyClass = SafetyClassSafe
		default:
			spec.SafetyClass = SafetyClassModerate
		}
	}
	if spec.SafetyClass == SafetyClassDestructive {
		spec.Destructive = true
	}
	if spec.MaxTimeoutS <= 0 {
		spec.MaxTimeoutS = 300
	}
	if spec.DefaultTimeoutS <= 0 {
		spec.DefaultTimeoutS = min(30, spec.MaxTimeoutS)
	}
	if spec.DefaultTimeoutS > spec.MaxTimeoutS {
		spec.DefaultTimeoutS = spec.MaxTimeoutS
	}
	if spec.MaxResultChars <= 0 {
		spec.MaxResultChars = 64 * 1024
	}
	if len(spec.AllowedModes) == 0 {
		spec.AllowedModes = defaultAllowedModes(spec.Name)
	}

	seen := map[planpkg.AgentMode]struct{}{}
	modes := make([]planpkg.AgentMode, 0, len(spec.AllowedModes))
	for _, mode := range spec.AllowedModes {
		normalized := planpkg.NormalizeMode(string(mode))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		modes = append(modes, normalized)
	}
	sort.Slice(modes, func(i, j int) bool { return modes[i] < modes[j] })
	spec.AllowedModes = modes

	if spec.ReadOnly {
		spec.Destructive = false
	}
	return spec
}

func ValidateToolSpec(spec ToolSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("tool spec name is required")
	}
	switch spec.SafetyClass {
	case SafetyClassSafe, SafetyClassModerate, SafetyClassSensitive, SafetyClassDestructive:
	default:
		return fmt.Errorf("tool %q has unsupported safety class %q", spec.Name, spec.SafetyClass)
	}
	if len(spec.AllowedModes) == 0 {
		return fmt.Errorf("tool %q must declare at least one allowed mode", spec.Name)
	}
	if spec.DefaultTimeoutS <= 0 {
		return fmt.Errorf("tool %q default timeout must be positive", spec.Name)
	}
	if spec.MaxTimeoutS < spec.DefaultTimeoutS {
		return fmt.Errorf("tool %q max timeout must be >= default timeout", spec.Name)
	}
	if spec.MaxResultChars <= 0 {
		return fmt.Errorf("tool %q max result chars must be positive", spec.Name)
	}
	if spec.ReadOnly && spec.Destructive {
		return fmt.Errorf("tool %q cannot be both read-only and destructive", spec.Name)
	}
	return nil
}

func modeAllowed(spec ToolSpec, mode planpkg.AgentMode) bool {
	normalized := planpkg.NormalizeMode(string(mode))
	for _, candidate := range spec.AllowedModes {
		if candidate == normalized {
			return true
		}
	}
	return false
}

func defaultAllowedModes(name string) []planpkg.AgentMode {
	switch name {
	case "list_files", "read_file", "search_text", "web_search", "web_fetch", "update_plan", "run_shell":
		return []planpkg.AgentMode{planpkg.ModeBuild, planpkg.ModePlan}
	default:
		return []planpkg.AgentMode{planpkg.ModeBuild}
	}
}
