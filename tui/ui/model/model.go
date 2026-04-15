package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.updateThinkingCard()
		m.refreshViewport()
		return m, cmd
	case agentEventMsg:
		m.handleAgentEvent(msg.Event)
		m.refreshViewport()
		return m, waitForAsync(m.async)
	case runFinishedMsg:
		if msg.RunID > 0 && msg.RunID != m.activeRunID {
			return m, waitForAsync(m.async)
		}
		m.busy = false
		m.runCancel = nil
		m.activeRunID = 0
		m.interruptSafe = false
		shouldResumeBTW := m.interrupting && len(m.pendingBTW) > 0
		m.interrupting = false
		finishReason := classifyRunFinish(msg.Err, shouldResumeBTW)
		if shouldResumeBTW {
			updateScope := formatBTWUpdateScope(len(m.pendingBTW))
			prompt := composeBTWPrompt(m.pendingBTW)
			m.pendingBTW = nil
			note := fmt.Sprintf("BTW accepted. Restarting with %s...", updateScope)
			if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
				note = fmt.Sprintf("Previous run ended early. Restarting with %s from BTW...", updateScope)
			}
			m.appendChat(chatEntry{
				Kind:   "system",
				Title:  "System",
				Body:   fmt.Sprintf("BTW interrupt accepted. Restarting with %s.", updateScope),
				Status: "final",
			})
			return m, m.beginRun(prompt, string(m.mode), note)
		}
		m.pendingBTW = nil
		switch finishReason {
		case runFinishReasonCompleted:
			m.runFinishedAt = time.Now()
			m.traceCollapsed = true
			if strings.TrimSpace(m.bufferedAssistantText) != "" {
				m.finishAssistantMessage(m.bufferedAssistantText)
				m.bufferedAssistantText = ""
			}
			if !m.shouldKeepStreamingIndexOnRunFinished() {
				m.streamingIndex = -1
			}
			elapsed := int(m.runFinishedAt.Sub(m.thinkingStartedAt).Round(time.Second).Seconds())
			if elapsed < 0 {
				elapsed = 0
			}
			m.statusNote = fmt.Sprintf("Completed ✓ (%ds)", elapsed)
			m.phase = "idle"
		case runFinishReasonCanceled:
			m.streamingIndex = -1
			m.bufferedAssistantText = ""
			m.statusNote = "Run canceled."
			m.phase = "idle"
			m.llmConnected = true
		case runFinishReasonFailed:
			m.streamingIndex = -1
			m.bufferedAssistantText = ""
			m.statusNote = "Run failed: " + msg.Err.Error()
			m.phase = "error"
			m.llmConnected = false
			m.failLatestAssistant(msg.Err.Error())
		default:
			m.streamingIndex = -1
			m.bufferedAssistantText = ""
			m.statusNote = "Ready."
			m.phase = "idle"
		}
		m.refreshViewport()
		return m, tea.Batch(waitForAsync(m.async), m.loadSessionsCmd())
	case approvalRequestMsg:
		m.approval = &approvalPrompt{
			Command: msg.Request.Command,
			Reason:  msg.Request.Reason,
			Reply:   msg.Reply,
		}
		m.statusNote = "Approval required."
		m.phase = "approval"
		return m, waitForAsync(m.async)
	case sessionsLoadedMsg:
		if msg.Err == nil {
			m.sessions = msg.Summaries
			if m.sessionCursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.sessionCursor = len(m.sessions) - 1
			}
		}
		return m, nil
	case tokenUsagePulledMsg:
		// Account-level usage is not session-accurate; ignore in session-only mode.
		return m, nil
	case selectionToastExpiredMsg:
		if msg.ID == m.selectionToastID {
			m.selectionToast = ""
		}
		return m, nil
	case mouseSelectionScrollTickMsg:
		return m.handleMouseSelectionScrollTick(msg)
	case tokenMonitorTickMsg:
		cmd, _ := m.tokenUsage.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if !m.sessionsOpen && !m.helpOpen && !m.commandOpen && m.approval == nil {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), "")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	return m, nil
}

func (m *model) ensureViewportMouse() {
	m.viewport.MouseWheelEnabled = true
	if m.viewport.MouseWheelDelta <= 0 {
		m.viewport.MouseWheelDelta = scrollStep
	}
}

func (m *model) ensurePlanMouse() {
	m.planView.MouseWheelEnabled = true
	if m.planView.MouseWheelDelta <= 0 {
		m.planView.MouseWheelDelta = scrollStep
	}
}

func (m model) shouldSuppressEnterAfterPaste() bool {
	if m.lastPasteAt.IsZero() {
		return false
	}
	if time.Since(m.lastPasteAt) > pasteSubmitGuard {
		return false
	}
	if strings.Contains(m.input.Value(), "\n") {
		return true
	}
	return time.Since(m.lastInputAt) <= 120*time.Millisecond
}
