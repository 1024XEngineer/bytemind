package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bytemind/internal/llm"
	"bytemind/internal/session"
)

func (m *model) applyUsage(usage llm.Usage) {
	m.tokenHasOfficialUsage = true
	input := max(0, usage.InputTokens)
	output := max(0, usage.OutputTokens)
	context := max(0, usage.ContextTokens)
	used := usage.TotalTokens
	if used == 0 {
		used = input + output + context
	}
	used = max(0, used)
	if used == 0 && input == 0 && output == 0 && context == 0 {
		return
	}

	// Replace provisional stream estimate with provider-confirmed usage.
	if m.tempEstimatedOutput > 0 {
		m.tokenUsedTotal = max(0, m.tokenUsedTotal-m.tempEstimatedOutput)
		m.tokenOutput = max(0, m.tokenOutput-m.tempEstimatedOutput)
	}
	m.tempEstimatedOutput = 0

	m.tokenUsedTotal += used
	m.tokenInput += input
	m.tokenOutput += output
	m.tokenContext += context
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(false)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
}

func (m *model) SetUsage(used, total int) tea.Cmd {
	m.tokenHasOfficialUsage = true
	m.tokenUsage.SetUnavailable(false)
	return m.tokenUsage.SetUsage(used, 0)
}

func (m model) renderStartupGuidePanel() string {
	width := max(24, m.commandPaletteWidth())
	title := strings.TrimSpace(m.startupGuide.Title)
	if title == "" {
		title = "Provider setup required"
	}
	status := strings.TrimSpace(m.startupGuide.Status)
	if status == "" {
		status = "AI provider is not available."
	}

	innerWidth := max(20, width-commandPaletteStyle.GetHorizontalFrameSize())
	content := make([]string, 0, 2+len(m.startupGuide.Lines))
	content = append(content, accentStyle.Render(title))
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(status))
	for _, line := range m.startupGuide.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(line))
	}
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(startupGuideInputHint(m.startupGuide.CurrentField)))

	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
}

func (m model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil && m.runtime == nil {
			return sessionsLoadedMsg{}
		}
		api := m.runtimeAPI()
		if api == nil {
			return sessionsLoadedMsg{}
		}
		summaries, err := api.ListSessions(m.sessionLimit)
		return sessionsLoadedMsg{Summaries: summaries, Err: err}
	}
}

func (m model) fetchRemoteTokenUsageCmd() tea.Cmd {
	return func() tea.Msg {
		usage, err := fetchCurrentMonthUsage(m.cfg)
		if err != nil {
			return tokenUsagePulledMsg{Err: err}
		}
		return tokenUsagePulledMsg{
			Used:    usage.Used,
			Input:   usage.Input,
			Output:  usage.Output,
			Context: usage.Context,
		}
	}
}

func (m *model) restoreTokenUsageFromSession(sess *session.Session) {
	m.tempEstimatedOutput = 0
	m.tokenHasOfficialUsage = false
	m.tokenUsedTotal = 0
	m.tokenInput = 0
	m.tokenOutput = 0
	m.tokenContext = 0

	if sess != nil {
		m.accumulateTokenUsage(sess.Messages)
	}
}

func (m *model) accumulateTokenUsage(messages []llm.Message) {
	for _, msg := range messages {
		if msg.Usage == nil {
			continue
		}
		m.tokenHasOfficialUsage = true
		used := msg.Usage.TotalTokens
		if used <= 0 {
			used = msg.Usage.InputTokens + msg.Usage.OutputTokens + msg.Usage.ContextTokens
		}
		m.tokenUsedTotal += max(0, used)
		m.tokenInput += max(0, msg.Usage.InputTokens)
		m.tokenOutput += max(0, msg.Usage.OutputTokens)
		m.tokenContext += max(0, msg.Usage.ContextTokens)
	}
}
