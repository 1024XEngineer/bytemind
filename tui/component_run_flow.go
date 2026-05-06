package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/history"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"

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
	m.lastRunDuration = 0
	m.runIndicatorState = runIndicatorRunning
	m.chatAutoFollow = true
	m.suppressedAssistantDelta = ""
	spinnerTick := m.resetThinkingSpinner()
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
	return m.submitPreparedPrompt(promptInput, displayText)
}

func (m model) submitPreparedPrompt(promptInput RunPromptInput, displayText string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(promptInput.DisplayText) == "" && strings.TrimSpace(displayText) != "" {
		promptInput.DisplayText = displayText
	}
	m.closePlanActionPicker()
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

	if m.runCancel == nil {
		prompt := composeBTWPrompt(m.pendingBTW)
		m.pendingBTW = nil
		m.interrupting = false
		m.interruptSafe = false
		m.pendingInterrupt = false
		m.pendingInterruptReason = ""
		return m, m.beginRun(prompt, string(m.mode), "BTW accepted. Restarting with your update...")
	}
	if m.requestRunInterrupt("btw") {
		if m.pendingInterrupt {
			m.statusNote = "BTW queued. Waiting for current tool step to finish..."
		} else {
			m.statusNote = "BTW received. Stopping current run..."
		}
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return m, nil
}

func (m *model) requestRunInterrupt(source string) bool {
	if m.interrupting {
		if strings.EqualFold(strings.TrimSpace(source), "esc") {
			m.statusNote = "Interrupt already requested. Waiting for current run to stop..."
		}
		return true
	}
	if m.runCancel == nil || !m.busy {
		return false
	}

	wasToolPhase := strings.EqualFold(strings.TrimSpace(m.phase), "tool")
	m.interrupting = true
	m.phase = "interrupting"
	m.pendingInterruptReason = strings.TrimSpace(source)
	if wasToolPhase {
		m.interruptSafe = true
		m.pendingInterrupt = true
		if strings.EqualFold(strings.TrimSpace(source), "esc") {
			m.statusNote = "Interrupt requested. Waiting for current tool step to finish..."
		}
		return true
	}

	m.interruptSafe = false
	m.pendingInterrupt = false
	m.pendingInterruptReason = ""
	if strings.EqualFold(strings.TrimSpace(source), "esc") {
		m.statusNote = "Interrupt requested. Stopping current run..."
	}
	m.runCancel()
	return true
}

func (m *model) handleAgentEvent(event Event) {
	if event.AgentID != "" {
		m.handleSubAgentEvent(event)
		return
	}
	switch event.Type {
	case EventRunStarted:
		m.tempEstimatedOutput = 0
		m.lastTokenReceivedAt = time.Now()
	case EventAssistantDelta:
		m.phase = "responding"
		m.statusNote = "LLM is responding..."
		m.llmConnected = true
		m.lastTokenReceivedAt = time.Now()
		m.appendAssistantDelta(event.Content)
	case EventAssistantMessage:
		m.llmConnected = true
		m.lastTokenReceivedAt = time.Now()
		m.finishAssistantMessage(event.Content)
	case EventToolCallStarted:
		m.phase = "tool"
		m.llmConnected = true
		m.lastTokenReceivedAt = time.Now()
		m.suppressedAssistantDelta = ""
		// Demote previously running tools to queued
		for i := range m.chatItems {
			if m.chatItems[i].Kind == "tool" && m.chatItems[i].Status == "running" {
				m.chatItems[i].Status = "queued"
			}
		}
		m.finalizeAssistantTurnForTool(event.ToolName)
		m.populateLatestThinkingToolStep(event.ToolName, "", "running")
		renderer := GetToolRenderer(event.ToolName)
		label := "TOOL"
		if renderer != nil {
			label = renderer.DisplayLabel()
		} else {
			label = toolDisplayLabel(event.ToolName)
		}
		title := label + " | " + event.ToolName
		compactBody, detailLines := summarizeToolCallStart(event.ToolName, event.ToolArguments)
		m.appendChat(chatEntry{
			Kind:        "tool",
			Title:       title,
			Body:        "",
			Status:      "running",
			CompactBody: compactBody,
			DetailLines: detailLines,
			ToolCallID:  event.ToolCallID,
		})
		m.statusNote = "Running tool: " + event.ToolName
	case EventToolCallCompleted:
		rendered := renderToolPayload(event.ToolName, event.ToolResult)
		summary := rendered.Summary
		lines := rendered.DetailLines
		status := rendered.Status
		compactBody := rendered.CompactLine
		m.finishToolCall(event.ToolCallID, event.ToolName, joinSummary(summary, lines), status, compactBody, lines)
		m.statusNote = summary
		m.phase = "thinking"
		if m.interruptSafe && m.interrupting && m.pendingInterrupt && m.runCancel != nil {
			m.interruptSafe = false
			m.pendingInterrupt = false
			m.pendingInterruptReason = ""
			m.phase = "interrupting"
			m.statusNote = "Interrupt requested. Stopping current run..."
			m.runCancel()
		}
	case EventPlanUpdated:
		m.plan = copyPlanState(event.Plan)
		m.phase = string(planpkg.NormalizePhase(string(m.plan.Phase)))
		if m.phase == "none" {
			m.phase = "plan"
		}
		switch {
		case planpkg.HasActiveChoice(m.plan):
			m.statusNote = "Plan updated. A clarification choice will appear after this reply finishes."
		case canContinuePlan(m.plan):
			m.statusNote = "Plan converged. Review the full plan, then choose the next action from the picker."
		case len(m.plan.DecisionGaps) > 0:
			m.statusNote = fmt.Sprintf("Plan updated. %d decision gap(s) remain.", len(m.plan.DecisionGaps))
		default:
			m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan.Steps))
		}
		if !hasRenderablePlanAction(m.plan) || m.busy {
			m.closePlanActionPicker()
		} else {
			m.syncPlanActionPicker()
		}
	case EventUsageUpdated:
		m.applyUsage(event.Usage)
	case EventRunFinished:
		if strings.TrimSpace(event.Content) != "" {
			m.statusNote = "Run finished."
		}
		m.phase = "idle"
		if m.mode == modePlan && hasRenderablePlanAction(m.plan) {
			m.syncPlanActionPicker()
			m.statusNote = planActionStatusNote(m.plan)
		} else {
			m.closePlanActionPicker()
		}
	}
}

func summarizeToolCallStart(toolName, rawArgs string) (string, []string) {
	switch strings.TrimSpace(strings.ToLower(toolName)) {
	case "search_text", "web_search":
		var args struct {
			Query string `json:"query"`
		}
		if json.Unmarshal([]byte(rawArgs), &args) == nil {
			query := strings.TrimSpace(args.Query)
			if query != "" {
				return fmt.Sprintf("%q", query), []string{"query: " + query}
			}
		}
	case "read_file":
		var args struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		if json.Unmarshal([]byte(rawArgs), &args) == nil {
			path := strings.TrimSpace(args.Path)
			if path != "" {
				name := filepath.ToSlash(path)
				if args.StartLine > 0 || args.EndLine > 0 {
					return fmt.Sprintf("%s (%d-%d)", name, args.StartLine, args.EndLine), []string{
						"path: " + name,
						fmt.Sprintf("range: %d-%d", args.StartLine, args.EndLine),
					}
				}
				return name, []string{"path: " + name}
			}
		}
	case "list_files":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(rawArgs), &args) == nil {
			path := strings.TrimSpace(args.Path)
			if path == "" {
				path = "."
			}
			path = filepath.ToSlash(path)
			return path, []string{"path: " + path}
		}
	}
	return "", nil
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

func (m *model) handleSubAgentEvent(event Event) {
	switch event.Type {
	case EventAssistantDelta:
		delta := stripStreamControlTags(event.Content)
		if delta == "" {
			return
		}
		if len(m.subAgentStreamItems) > 0 {
			last := &m.subAgentStreamItems[len(m.subAgentStreamItems)-1]
			if last.Kind == "assistant" && last.Status == "streaming" {
				last.Body += delta
				return
			}
		}
		m.subAgentStreamItems = append(m.subAgentStreamItems, chatEntry{
			Kind:   "assistant",
			Title:  event.AgentID,
			Body:   delta,
			Status: "streaming",
		})
	case EventToolCallStarted:
		m.subAgentStreamItems = append(m.subAgentStreamItems, chatEntry{
			Kind:   "tool",
			Title:  toolEntryTitle(event.ToolName),
			Body:   "",
			Status: "running",
		})
	case EventToolCallCompleted:
		rendered := renderToolPayload(event.ToolName, event.ToolResult)
		summary := rendered.Summary
		lines := rendered.DetailLines
		status := rendered.Status
		body := joinSummary(summary, lines)
		for i := len(m.subAgentStreamItems) - 1; i >= 0; i-- {
			item := &m.subAgentStreamItems[i]
			if item.Kind == "tool" && item.Status == "running" {
				item.Body = body
				item.Status = status
				return
			}
		}
		m.subAgentStreamItems = append(m.subAgentStreamItems, chatEntry{
			Kind:   "tool",
			Title:  toolEntryTitle(event.ToolName),
			Body:   body,
			Status: status,
		})
	}
}
