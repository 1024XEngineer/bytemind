package tui

import (
	"context"
	"fmt"
	"strings"

	"bytemind/internal/session"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
)

type subAgentCommandResultMsg struct {
	Input  string
	Result tools.DelegateSubAgentResult
	Err    error
}

type subAgentCommandRunner interface {
	ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic)
	FindSubAgent(name string) (subagentspkg.Agent, bool)
	FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool)
	DispatchSubAgent(ctx context.Context, sess *session.Session, mode string, request tools.DelegateSubAgentRequest) (tools.DelegateSubAgentResult, error)
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

	alias := strings.ToLower(strings.TrimSpace(builtinName))
	switch alias {
	case "/exploer":
		alias = "/explorer"
	}

	agent, ok := runner.FindBuiltinSubAgent(alias)
	if !ok {
		m.appendCommandExchange(input, fmt.Sprintf("builtin subagent is unavailable: %s", builtinName))
		m.statusNote = "Builtin subagent unavailable."
		return nil
	}

	task := extractSubAgentTaskInput(input)
	if task == "" {
		usage := fmt.Sprintf("usage: %s <task>\nTip: use /agents %s to inspect the definition first.", alias, agent.Name)
		m.appendCommandExchange(input, usage)
		m.statusNote = "Subagent task is required."
		return nil
	}

	request := tools.DelegateSubAgentRequest{
		Agent: agent.Name,
		Task:  task,
	}
	if m.async != nil {
		return m.runBuiltinSubAgentCommandAsync(input, agent.Name, runner, request)
	}

	result, dispatchErr := runner.DispatchSubAgent(context.Background(), m.sess, string(modeBuild), request)
	if dispatchErr != nil {
		return dispatchErr
	}

	m.appendCommandExchange(input, renderSubAgentDispatchResult(result))
	if result.OK {
		m.statusNote = fmt.Sprintf("Subagent `%s` completed.", result.Agent)
		return nil
	}
	m.statusNote = fmt.Sprintf("Subagent `%s` failed.", agent.Name)
	return nil
}

func (m *model) runBuiltinSubAgentCommandAsync(
	input string,
	agentName string,
	runner subAgentCommandRunner,
	request tools.DelegateSubAgentRequest,
) error {
	if m == nil {
		return fmt.Errorf("model is unavailable")
	}
	if m.subAgentCommandPending {
		return fmt.Errorf("a subagent command is already running")
	}
	if m.async == nil {
		return fmt.Errorf("subagent async channel is unavailable")
	}

	m.subAgentCommandPending = true
	m.statusNote = fmt.Sprintf("Subagent `%s` running...", strings.TrimSpace(agentName))

	asyncCh := m.async
	commandInput := strings.TrimSpace(input)
	parentSession := m.sess

	go func() {
		result, dispatchErr := runner.DispatchSubAgent(context.Background(), parentSession, string(modeBuild), request)
		asyncCh <- subAgentCommandResultMsg{
			Input:  commandInput,
			Result: result,
			Err:    dispatchErr,
		}
	}()
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

func extractSubAgentTaskInput(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return ""
	}
	head := strings.TrimSpace(fields[0])
	remainder := strings.TrimSpace(strings.TrimPrefix(raw, head))
	return strings.TrimSpace(remainder)
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

func renderSubAgentDispatchResult(result tools.DelegateSubAgentResult) string {
	lines := make([]string, 0, 12)
	if result.OK {
		status := strings.TrimSpace(result.Status)
		if status == "" {
			status = "completed"
		}
		lines = append(lines, fmt.Sprintf("subagent %s %s", strings.TrimSpace(result.Agent), status))
		if taskID := strings.TrimSpace(result.TaskID); taskID != "" {
			lines = append(lines, fmt.Sprintf("task_id %s", taskID))
		}
		if invocationID := strings.TrimSpace(result.InvocationID); invocationID != "" {
			lines = append(lines, fmt.Sprintf("invocation %s", invocationID))
		}
		summary := strings.TrimSpace(result.Summary)
		if summary == "" {
			summary = "SubAgent task completed."
		}
		lines = append(lines, "summary "+summary)
		if len(result.Findings) > 0 {
			lines = append(lines, fmt.Sprintf("findings %d", len(result.Findings)))
			for _, finding := range result.Findings {
				title := strings.TrimSpace(finding.Title)
				body := strings.TrimSpace(finding.Body)
				switch {
				case title != "" && body != "":
					lines = append(lines, fmt.Sprintf("- %s: %s", title, body))
				case title != "":
					lines = append(lines, fmt.Sprintf("- %s", title))
				case body != "":
					lines = append(lines, fmt.Sprintf("- %s", body))
				}
			}
		}
		if len(result.References) > 0 {
			lines = append(lines, fmt.Sprintf("references %d", len(result.References)))
			for _, ref := range result.References {
				path := strings.TrimSpace(ref.Path)
				if path == "" {
					continue
				}
				line := ""
				if ref.Line > 0 {
					line = fmt.Sprintf(":%d", ref.Line)
				}
				note := strings.TrimSpace(ref.Note)
				if note != "" {
					lines = append(lines, fmt.Sprintf("- %s%s (%s)", path, line, note))
				} else {
					lines = append(lines, fmt.Sprintf("- %s%s", path, line))
				}
			}
		}
		return strings.Join(lines, "\n")
	}

	lines = append(lines, fmt.Sprintf("subagent %s failed", strings.TrimSpace(result.Agent)))
	if invocationID := strings.TrimSpace(result.InvocationID); invocationID != "" {
		lines = append(lines, fmt.Sprintf("invocation %s", invocationID))
	}
	if taskID := strings.TrimSpace(result.TaskID); taskID != "" {
		lines = append(lines, fmt.Sprintf("task_id %s", taskID))
	}
	if result.Error != nil {
		code := strings.TrimSpace(result.Error.Code)
		message := strings.TrimSpace(result.Error.Message)
		if code != "" {
			lines = append(lines, "error_code "+code)
		}
		if message != "" {
			lines = append(lines, "error "+message)
		}
	}
	if len(lines) == 1 {
		lines = append(lines, "error subagent execution failed")
	}
	return strings.Join(lines, "\n")
}
