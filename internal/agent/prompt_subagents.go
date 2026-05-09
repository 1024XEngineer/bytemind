package agent

import (
	"sort"
	"strings"
)

func (r *Runner) promptSubAgents() []PromptSubAgent {
	if r.subAgentManager == nil {
		return nil
	}
	agents, _ := r.subAgentManager.List()
	if len(agents) == 0 {
		return nil
	}
	out := make([]PromptSubAgent, 0, len(agents))
	for _, item := range agents {
		name := strings.TrimSpace(item.Name)
		description := strings.TrimSpace(item.Description)
		if name == "" || description == "" {
			continue
		}
		out = append(out, PromptSubAgent{
			Name:        name,
			Description: description,
			WhenToUse:   strings.TrimSpace(item.WhenToUse),
			Mode:        strings.TrimSpace(item.Mode),
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
