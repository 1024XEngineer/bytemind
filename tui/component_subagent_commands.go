package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/1024XEngineer/bytemind/internal/session"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
	"github.com/charmbracelet/lipgloss"
)

type subAgentCommandRunner interface {
	ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic)
	FindSubAgent(name string) (subagentspkg.Agent, bool)
	FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool)
	DispatchSubAgent(ctx context.Context, sess *session.Session, mode string, request tools.DelegateSubAgentRequest) (tools.DelegateSubAgentResult, error)
}

const builtinSubAgentRequestTimeout = "90s"

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
		m.appendCommandExchange(input, fmt.Sprintf("builtin subagent is unavailable: %s", alias))
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

	return m.submitBuiltinSubAgentPreference(input, agent.Name, task)
}

func (m *model) submitBuiltinSubAgentPreference(input, agentName, task string) error {
	if m == nil {
		return fmt.Errorf("model is unavailable")
	}

	runner, err := m.requireSubAgentRunner()
	if err != nil {
		return err
	}

	normalizedInput := strings.TrimSpace(input)
	if normalizedInput == "" {
		normalizedInput = fmt.Sprintf("/%s %s", strings.TrimSpace(agentName), strings.TrimSpace(task))
	}

	m.screen = screenChat
	m.input.Reset()
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   normalizedInput,
		Status: "final",
	})
	m.busy = true
	m.subAgentPending = true
	m.subAgentName = agentName
	m.subAgentTask = task
	m.phase = "thinking"
	m.runStartedAt = time.Now()
	m.lastRunDuration = 0
	m.runIndicatorState = runIndicatorRunning
	m.pendingCommandCmd = m.resetThinkingSpinner()
	m.statusNote = fmt.Sprintf("Running subagent %s...", agentName)

	// Add styled progress card to conversation
	progressCard := renderSubAgentProgressCard(agentName, task, m.spinner.View(), "0s", max(40, m.width-8))
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   progressCard,
		Status: "thinking",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.chatAutoFollow = true
	m.refreshViewport()

	asyncCh := m.async
	commandInput := normalizedInput
	dispatchAgent := agentName
	dispatchTask := task
	sess := m.sess
	mode := string(m.mode)
	resultWidth := max(40, m.width-8)

	go func() {
		result, dispatchErr := runner.DispatchSubAgent(
			context.Background(), sess, mode,
			tools.DelegateSubAgentRequest{
				Agent:   dispatchAgent,
				Task:    dispatchTask,
				Timeout: builtinSubAgentRequestTimeout,
			},
		)
		if dispatchErr != nil {
			asyncCh <- subAgentResultMsg{
				Input: commandInput,
				Err:   dispatchErr,
			}
			return
		}
		asyncCh <- subAgentResultMsg{
			Input:    commandInput,
			Response: renderSubAgentResultCard(result, resultWidth),
			Status:   fmt.Sprintf("Subagent %s completed.", dispatchAgent),
		}
	}()
	return nil
}

func normalizeBuiltinSubAgentCommandInput(input string) (string, string, bool) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", "", false
	}
	lower := strings.ToLower(raw)
	commands := []string{"/explorer", "/exploer", "/review"}
	for _, command := range commands {
		if !strings.HasPrefix(lower, command) {
			continue
		}
		if len(raw) == len(command) {
			return raw, command, true
		}
		suffix := raw[len(command):]
		if suffix == "" {
			return raw, command, true
		}
		r, _ := utf8.DecodeRuneInString(suffix)
		if unicode.IsSpace(r) {
			return raw, command, true
		}
		if !isSlashCommandIdentifierRune(r) {
			return command + " " + strings.TrimSpace(suffix), command, true
		}
	}
	return "", "", false
}

func isSlashCommandIdentifierRune(r rune) bool {
	if r == utf8.RuneError {
		return false
	}
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return true
	}
	switch r {
	case '-', '_', '.', ':':
		return true
	default:
		return false
	}
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
	return renderSubAgentResultCard(result, 80)
}

func renderSubAgentResultCard(result tools.DelegateSubAgentResult, width int) string {
	if width < 40 {
		width = 40
	}
	innerWidth := width - 4

	var sections []string

	// Header: agent name + status badge
	agentName := strings.TrimSpace(result.Agent)
	if agentName == "" {
		agentName = "subagent"
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		if result.OK {
			status = "completed"
		} else {
			status = "failed"
		}
	}
	badgeType := subAgentStatusBadgeType(status)
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		strongStyle.Render("SubAgent "),
		accentStyle.Bold(true).Render(agentName),
		lipgloss.NewStyle().Render("  "),
		renderPillBadge(strings.ToUpper(status), badgeType),
	)
	sections = append(sections, header)

	// Metadata line
	var meta []string
	if taskID := strings.TrimSpace(result.TaskID); taskID != "" {
		meta = append(meta, "task: "+taskID)
	}
	if invocationID := strings.TrimSpace(result.InvocationID); invocationID != "" {
		meta = append(meta, "invocation: "+invocationID)
	}
	if len(meta) > 0 {
		sections = append(sections, mutedStyle.Render(strings.Join(meta, "  |  ")))
	}

	// Divider
	divider := strings.Repeat("─", innerWidth)
	sections = append(sections, mutedStyle.Render(divider))

	// Error section
	if !result.OK && result.Error != nil {
		code := strings.TrimSpace(result.Error.Code)
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			message = "subagent execution failed"
		}
		errLine := errorStyle.Render("Error: ")
		if code != "" {
			errLine += mutedStyle.Render("[" + code + "] ")
		}
		errLine += errorStyle.Render(message)
		sections = append(sections, errLine)
	}

	// Summary
	if result.OK {
		summary := strings.TrimSpace(result.Summary)
		if summary == "" {
			summary = "SubAgent task completed."
		}
		sections = append(sections, wrapText(summary, innerWidth))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(subAgentBorderAccent(status)).
		Background(semanticColors.PanelMuted).
		Padding(0, 1).
		Width(width).
		Render(content)
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%ds", s/60, s%60)
}

func renderSubAgentProgressCard(agentName, task, spinner, elapsed string, width int) string {
	if width < 40 {
		width = 40
	}
	innerWidth := width - 4

	var sections []string

	// Header: spinner + agent name
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Render(spinner+" "),
		accentStyle.Bold(true).Render(agentName),
	)
	sections = append(sections, header)

	// Task snippet (truncated)
	taskSnippet := strings.TrimSpace(task)
	if taskSnippet != "" {
		if len(taskSnippet) > innerWidth*2 {
			taskSnippet = taskSnippet[:innerWidth*2-3] + "..."
		}
		sections = append(sections, mutedStyle.Render(wrapText(taskSnippet, innerWidth)))
	}

	// Elapsed time
	if elapsed != "" {
		sections = append(sections, mutedStyle.Render("Elapsed: "+elapsed))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(semanticColors.Accent).
		Background(semanticColors.PanelMuted).
		Padding(0, 1).
		Width(width).
		Render(content)
}

func subAgentStatusBadgeType(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "success"
	case "failed":
		return "error"
	case "accepted", "queued", "running":
		return "accent"
	default:
		return "neutral"
	}
}

func subAgentBorderAccent(status string) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return semanticColors.Success
	case "failed":
		return semanticColors.Danger
	case "accepted", "queued", "running":
		return semanticColors.Accent
	default:
		return semanticColors.Border
	}
}

func wrapText(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	var lines []string
	for len(text) > 0 {
		if len(text) <= width {
			lines = append(lines, text)
			break
		}
		cut := width
		for cut > 0 && text[cut] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = width
		}
		lines = append(lines, text[:cut])
		text = strings.TrimSpace(text[cut:])
	}
	return strings.Join(lines, "\n")
}
