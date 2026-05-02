package agent

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func (r *Runner) promptSkills() []PromptSkill {
	if r.skillManager == nil {
		return nil
	}
	skillList, _ := r.skillManager.List()
	if len(skillList) == 0 {
		return nil
	}
	out := make([]PromptSkill, 0, len(skillList))
	for _, skill := range skillList {
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if name == "" || description == "" {
			continue
		}
		out = append(out, PromptSkill{
			Name:        name,
			Description: description,
			Enabled:     true,
		})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func toolNames(definitions []llm.ToolDefinition) []string {
	if len(definitions) == 0 {
		return nil
	}
	names := make([]string, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		name := strings.TrimSpace(definition.Function.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
