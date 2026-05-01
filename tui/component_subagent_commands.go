package tui

import (
	"fmt"
	"strings"

	subagentspkg "bytemind/internal/subagents"
)

type subAgentCommandRunner interface {
	ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic)
	FindSubAgent(name string) (subagentspkg.Agent, bool)
	FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool)
}

func (m *model) runAgentsCommand(input string, fields []string) error {
	runner, err := m.requireSubAgentRunner()
	if err != nil {
		return err
	}

	agents, _ := runner.ListSubAgents()
	if len(fields) > 1 {
		agent, ok := runner.FindSubAgent(fields[1])
		if !ok {
			m.appendCommandExchange(input, fmt.Sprintf("subagent not found: %s", fields[1]))
			m.statusNote = "Subagent not found."
			return nil
		}
		m.appendCommandExchange(input, renderSubAgentDetail(agent))
		m.statusNote = fmt.Sprintf("Opened subagent `%s`.", agent.Name)
		return nil
	}

	m.appendCommandExchange(input, renderSubAgentsView(agents))
	m.statusNote = fmt.Sprintf("Discovered %d subagent(s).", len(agents))
	return nil
}

func (m *model) runBuiltinSubAgentCommand(input, builtinName string) error {
	runner, err := m.requireSubAgentRunner()
	if err != nil {
		return err
	}

	agent, ok := runner.FindBuiltinSubAgent(builtinName)
	if !ok {
		m.appendCommandExchange(input, fmt.Sprintf("builtin subagent is unavailable: %s", builtinName))
		m.statusNote = "Builtin subagent unavailable."
		return nil
	}
	m.appendCommandExchange(input, renderSubAgentDetail(agent))
	m.statusNote = fmt.Sprintf("Opened builtin subagent `%s`.", agent.Name)
	return nil
}

func (m *model) requireSubAgentRunner() (subAgentCommandRunner, error) {
	if m.runner == nil {
		return nil, fmt.Errorf("runner is unavailable")
	}
	runner, ok := any(m.runner).(subAgentCommandRunner)
	if !ok {
		return nil, fmt.Errorf("subagent commands are unavailable in this build")
	}
	return runner, nil
}

func renderSubAgentsView(agents []subagentspkg.Agent) string {
	if len(agents) == 0 {
		return "No subagents available."
	}

	lines := make([]string, 0, len(agents)+1)
	lines = append(lines, "Available subagents:")
	for _, agent := range agents {
		description := strings.TrimSpace(agent.Description)
		if description == "" {
			description = "No description provided."
		}
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", agent.Name, agent.Scope, description))
	}
	return strings.Join(lines, "\n")
}

func renderSubAgentDetail(agent subagentspkg.Agent) string {
	lines := make([]string, 0, 10)
	lines = append(lines, fmt.Sprintf("subagent %s", agent.Name))
	lines = append(lines, fmt.Sprintf("scope %s", agent.Scope))
	lines = append(lines, fmt.Sprintf("entry %s", agent.Entry))
	if strings.TrimSpace(agent.Mode) != "" {
		lines = append(lines, fmt.Sprintf("mode %s", agent.Mode))
	}
	if strings.TrimSpace(agent.Output) != "" {
		lines = append(lines, fmt.Sprintf("output %s", agent.Output))
	}
	if strings.TrimSpace(agent.Isolation) != "" {
		lines = append(lines, fmt.Sprintf("isolation %s", agent.Isolation))
	}
	if len(agent.Tools) > 0 {
		lines = append(lines, fmt.Sprintf("tools %s", strings.Join(agent.Tools, ", ")))
	}
	if len(agent.DisallowedTools) > 0 {
		lines = append(lines, fmt.Sprintf("disallowed %s", strings.Join(agent.DisallowedTools, ", ")))
	}
	if source := strings.TrimSpace(agent.SourcePath); source != "" {
		lines = append(lines, fmt.Sprintf("source %s", source))
	}
	if description := strings.TrimSpace(agent.Description); description != "" {
		lines = append(lines, fmt.Sprintf("description %s", description))
	}
	return strings.Join(lines, "\n")
}
