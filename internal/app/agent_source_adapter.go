package app

import (
	"github.com/1024XEngineer/bytemind/internal/mention"
	"github.com/1024XEngineer/bytemind/tui"
)

type tuiRunnerAgentSource struct {
	runner tui.Runner
}

func (s *tuiRunnerAgentSource) ListAgents() []mention.AgentEntry {
	if s.runner == nil {
		return nil
	}
	mgr := s.runner.SubAgentManager()
	if mgr == nil {
		return nil
	}
	agents, _ := mgr.List()
	entries := make([]mention.AgentEntry, 0, len(agents))
	for _, a := range agents {
		entries = append(entries, mention.AgentEntry{
			Name:        a.Name,
			Description: a.Description,
			Scope:       string(a.Scope),
		})
	}
	return entries
}
