package tui

import (
	"fmt"
	"strings"
	"time"

	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
	"github.com/charmbracelet/lipgloss"
)

type subAgentCommandRunner interface {
	ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic)
	FindSubAgent(name string) (subagentspkg.Agent, bool)
	FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool)
}

func (m *model) runAgentsCommand(input string) error {
	runner, err := m.requireSubAgentRunner()
	if err != nil {
		return err
	}

	agents, _ := runner.ListSubAgents()
	m.appendCommandExchange(input, renderSubAgentsView(agents))
	m.statusNote = fmt.Sprintf("Discovered %d subagent(s).", len(agents))
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
