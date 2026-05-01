package agent

import subagentspkg "bytemind/internal/subagents"

func (r *Runner) ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic) {
	if r.subAgentManager == nil {
		return nil, nil
	}
	return r.subAgentManager.List()
}

func (r *Runner) FindSubAgent(name string) (subagentspkg.Agent, bool) {
	if r.subAgentManager == nil {
		return subagentspkg.Agent{}, false
	}
	return r.subAgentManager.Find(name)
}

func (r *Runner) FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool) {
	if r.subAgentManager == nil {
		return subagentspkg.Agent{}, false
	}
	return r.subAgentManager.FindBuiltin(name)
}
