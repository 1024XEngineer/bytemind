package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"bytemind/internal/history"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) beginRun(prompt, mode, note string) tea.Cmd {
	return m.beginRunWithInput(RunPromptInput{
		UserMessage: llm.NewUserTextMessage(prompt),
		DisplayText: prompt,
	}, mode, note)
}

func (m *model) beginRunWithInput(promptInput RunPromptInput, mode, note string) tea.Cmd {
	runCtx, cancel := context.WithCancel(context.Background())
	m.runSeq++
	runID := m.runSeq
	m.activeRunID = runID
	m.runCancel = cancel
	m.streamingIndex = -1
	if strings.TrimSpace(note) == "" {
		note = "Request sent to LLM. Waiting for response..."
	}
	m.statusNote = note
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.runStartedAt = time.Now()
	m.chatAutoFollow = true
	spinnerTick := m.resetThinkingSpinner()
	m.ensureThinkingCard()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return tea.Batch(m.startRunCmd(runCtx, runID, promptInput, mode), spinnerTick, waitForAsync(m.async))
}

func (m model) submitPrompt(value string) (tea.Model, tea.Cmd) {
	promptInput, displayText, err := m.buildPromptInput(value)
	if err != nil {
		m.statusNote = err.Error()
		return m, nil
	}

	m.input.Reset()
	m.clearPasteTransaction()
	m.clearVirtualPasteParts()
	m.screen = screenChat
	if m.promptHistoryLoaded {
		entry := history.PromptEntry{
			Timestamp: time.Now().UTC(),
			Workspace: strings.TrimSpace(m.workspace),
			Prompt:    strings.TrimSpace(displayText),
		}
		if m.sess != nil {
			entry.SessionID = m.sess.ID
		}
		if entry.Prompt != "" {
			m.promptHistoryEntries = append(m.promptHistoryEntries, entry)
			if len(m.promptHistoryEntries) > promptSearchLoadLimit {
				m.promptHistoryEntries = m.promptHistoryEntries[len(m.promptHistoryEntries)-promptSearchLoadLimit:]
			}
		}
	}
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   displayText,
		Status: "final",
	})
	return m, m.beginRunWithInput(promptInput, string(m.mode), "Request sent to LLM. Waiting for response...")
}

func (m model) submitBTW(value string) (tea.Model, tea.Cmd) {
	value = strings.TrimSpace(value)
	if value == "" {
		return m, nil
	}

	m.input.Reset()
	m.clearPasteTransaction()
	m.clearVirtualPasteParts()
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()) + " | btw",
		Body:   value,
		Status: "final",
	})
	var dropped int
	m.pendingBTW, dropped = queueBTWUpdate(m.pendingBTW, value)
	m.chatAutoFollow = true

	if m.interrupting {
		if dropped > 0 {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending, dropped %d older). Waiting for current run to stop...", len(m.pendingBTW), dropped)
		} else {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending). Waiting for current run to stop...", len(m.pendingBTW))
		}
		m.phase = "interrupting"
		if m.width > 0 && m.height > 0 {
			m.syncLayoutForCurrentScreen()
			m.refreshViewport()
		}
		return m, nil
	}

	wasToolPhase := m.phase == "tool"
	m.interrupting = true
	m.phase = "interrupting"
	if m.runCancel != nil {
		if wasToolPhase {
			m.interruptSafe = true
			m.statusNote = "BTW queued. Waiting for current tool step to finish..."
		} else {
			m.interruptSafe = false
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	} else {
		prompt := composeBTWPrompt(m.pendingBTW)
		m.pendingBTW = nil
		m.interrupting = false
		m.interruptSafe = false
		return m, m.beginRun(prompt, string(m.mode), "BTW accepted. Restarting with your update...")
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return m, nil
}

func (m *model) handleAgentEvent(event Event) {
	switch event.Type {
	case EventRunStarted:
		m.tempEstimatedOutput = 0
	case EventAssistantDelta:
		m.phase = "responding"
		m.statusNote = "LLM is responding..."
		m.llmConnected = true
		m.appendAssistantDelta(event.Content)
	case EventAssistantMessage:
		m.llmConnected = true
		m.finishAssistantMessage(event.Content)
	case EventToolCallStarted:
		m.phase = "tool"
		m.llmConnected = true
		m.finalizeAssistantTurnForTool(event.ToolName)
		m.populateLatestThinkingToolStep(event.ToolName, "", "running")
		m.appendChat(chatEntry{
			Kind:   "tool",
			Title:  toolEntryTitle(event.ToolName),
			Body:   "",
			Status: "running",
		})
		m.toolRuns = append(m.toolRuns, toolRun{
			Name:    event.ToolName,
			Summary: "Tool call started.",
			Status:  "running",
		})
		m.statusNote = "Running tool: " + event.ToolName
	case EventToolCallCompleted:
		summary, lines, status := summarizeTool(event.ToolName, event.ToolResult)
		m.finishLatestToolCall(event.ToolName, joinSummary(summary, lines), status)
		if len(m.toolRuns) > 0 {
			index := len(m.toolRuns) - 1
			m.toolRuns[index].Summary = summary
			m.toolRuns[index].Lines = lines
			m.toolRuns[index].Status = status
		}
		m.statusNote = summary
		m.phase = "thinking"
		if m.interruptSafe && m.interrupting && len(m.pendingBTW) > 0 && m.runCancel != nil {
			m.interruptSafe = false
			m.phase = "interrupting"
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	case EventPlanUpdated:
		m.plan = copyPlanState(event.Plan)
		m.phase = string(planpkg.NormalizePhase(string(m.plan.Phase)))
		if m.phase == "none" {
			m.phase = "plan"
		}
		m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan.Steps))
	case EventUsageUpdated:
		m.applyUsage(event.Usage)
	case EventRunFinished:
		if strings.TrimSpace(event.Content) != "" {
			m.statusNote = "Run finished."
		}
		m.phase = "idle"
	}
}

func (m model) startRunCmd(runCtx context.Context, runID int, prompt RunPromptInput, mode string) tea.Cmd {
	return func() tea.Msg {
		if m.runner == nil {
			m.async <- runFinishedMsg{RunID: runID, Err: fmt.Errorf("runner is unavailable")}
			return nil
		}
		go func() {
			_, err := m.runner.RunPromptWithInput(runCtx, m.sess, prompt, mode, io.Discard)
			m.async <- runFinishedMsg{RunID: runID, Err: err}
		}()
		return nil
	}
}
