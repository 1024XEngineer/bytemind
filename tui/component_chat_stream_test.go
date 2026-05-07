package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func TestAppendAssistantDeltaStripsTurnIntentTag(t *testing.T) {
	m := model{}
	m.appendAssistantDelta("<turn_intent>finalize</turn_intent>已收到，开始执行")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item, got %d", len(m.chatItems))
	}
	if got := m.chatItems[0].Body; got != "已收到，开始执行" {
		t.Fatalf("expected cleaned body, got %q", got)
	}
}

func TestFailLatestAssistantUpdatesTitleFromThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}
	m.failLatestAssistant("provider rate limited")

	last := m.chatItems[len(m.chatItems)-1]
	if last.Title != assistantLabel {
		t.Fatalf("expected title %q after error, got %q", assistantLabel, last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected status \"error\", got %q", last.Status)
	}
	if !strings.Contains(last.Body, "provider rate limited") {
		t.Fatalf("expected body to contain error text, got %q", last.Body)
	}
}

func TestFailLatestAssistantUpdatesTitleFromStreaming(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "partial response", Status: "streaming"},
		},
	}
	m.failLatestAssistant("connection reset")

	last := m.chatItems[0]
	if last.Title != assistantLabel {
		t.Fatalf("expected title %q after error, got %q", assistantLabel, last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected status \"error\", got %q", last.Status)
	}
}

func TestFailLatestAssistantCreatesEntryWhenNoAssistantItem(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
		},
	}
	m.failLatestAssistant("no response")

	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 chat items, got %d", len(m.chatItems))
	}
	last := m.chatItems[1]
	if last.Kind != "assistant" {
		t.Fatalf("expected assistant kind, got %q", last.Kind)
	}
	if last.Title != assistantLabel {
		t.Fatalf("expected title %q, got %q", assistantLabel, last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected status \"error\", got %q", last.Status)
	}
}

func TestFailLatestAssistantCreatesEntryWhenEmpty(t *testing.T) {
	m := model{}
	m.failLatestAssistant("timeout")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item, got %d", len(m.chatItems))
	}
	last := m.chatItems[0]
	if last.Title != assistantLabel {
		t.Fatalf("expected title %q, got %q", assistantLabel, last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected status \"error\", got %q", last.Status)
	}
	if !strings.Contains(last.Body, "timeout") {
		t.Fatalf("expected body to contain error text, got %q", last.Body)
	}
}

func TestFailLatestAssistantDefaultsToUnknownError(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}
	m.failLatestAssistant("  ")

	last := m.chatItems[0]
	if !strings.Contains(last.Body, "Unknown provider error") {
		t.Fatalf("expected default error text, got %q", last.Body)
	}
}

func TestFailLatestAssistantSkipsNonAssistantItems(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "tool", Title: "read_file", Body: "result", Status: "done"},
		},
	}
	m.failLatestAssistant("no assistant")

	if len(m.chatItems) != 3 {
		t.Fatalf("expected 3 chat items (appended new), got %d", len(m.chatItems))
	}
	last := m.chatItems[2]
	if last.Status != "error" {
		t.Fatalf("expected appended error entry, got %+v", last)
	}
}

func TestRemoveThinkingCardRemovesLastThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking...", Status: "thinking"},
		},
	}
	m.removeThinkingCard()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item after removal, got %d", len(m.chatItems))
	}
	if m.chatItems[0].Kind != "user" {
		t.Fatalf("expected remaining item to be user, got %q", m.chatItems[0].Kind)
	}
}

func TestRemoveThinkingCardRemovesPendingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "pending"},
		},
	}
	m.removeThinkingCard()

	if len(m.chatItems) != 0 {
		t.Fatalf("expected 0 chat items after removal, got %d", len(m.chatItems))
	}
}

func TestRemoveThinkingCardDoesNotRemoveFinalAssistant(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "answer", Status: "final"},
		},
	}
	m.removeThinkingCard()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item (not removed), got %d", len(m.chatItems))
	}
}

func TestRemoveThinkingCardDoesNotRemoveErrorAssistant(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "Request failed: error", Status: "error"},
		},
	}
	m.removeThinkingCard()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item (not removed), got %d", len(m.chatItems))
	}
}

func TestRemoveThinkingCardNoOpWhenEmpty(t *testing.T) {
	m := model{}
	m.removeThinkingCard()

	if len(m.chatItems) != 0 {
		t.Fatalf("expected 0 chat items, got %d", len(m.chatItems))
	}
}

func TestRunFailedConvertsThinkingCardToError(t *testing.T) {
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: 1,
		statusNote:     "Running...",
		phase:          "thinking",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "inspect repo", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited")})
	updated := got.(model)

	if updated.busy {
		t.Fatalf("expected failed run to clear busy state")
	}
	if updated.phase != "error" {
		t.Fatalf("expected phase \"error\", got %q", updated.phase)
	}
	if updated.llmConnected {
		t.Fatalf("expected llmConnected=false")
	}

	last := updated.chatItems[len(updated.chatItems)-1]
	if last.Title != assistantLabel {
		t.Fatalf("expected title %q after failure, got %q", assistantLabel, last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected status \"error\", got %q", last.Status)
	}
	if !strings.Contains(last.Body, "provider rate limited") {
		t.Fatalf("expected body to contain error text, got %q", last.Body)
	}
}

func TestRunFailedWithRateLimitErrorDisplaysCorrectly(t *testing.T) {
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: 1,
		statusNote:     "Request sent to LLM. Waiting for response...",
		phase:          "thinking",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "do something", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429 Too Many Requests")})
	updated := got.(model)

	if updated.phase != "error" {
		t.Fatalf("expected error phase, got %q", updated.phase)
	}
	if !strings.Contains(updated.statusNote, "rate limited") {
		t.Fatalf("expected rate limit in status note, got %q", updated.statusNote)
	}

	last := updated.chatItems[len(updated.chatItems)-1]
	if last.Title == thinkingLabel {
		t.Fatalf("error card should not keep thinking title, got %q", last.Title)
	}
	if last.Status != "error" {
		t.Fatalf("expected error status, got %q", last.Status)
	}
}

func TestRunCanceledDoesNotConvertThinkingCard(t *testing.T) {
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: 1,
		statusNote:     "Running...",
		phase:          "thinking",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "inspect repo", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: context.Canceled})
	updated := got.(model)

	if updated.phase != "idle" {
		t.Fatalf("expected idle phase after cancel, got %q", updated.phase)
	}
	// Thinking card should remain as-is after cancel (not converted to error)
	last := updated.chatItems[len(updated.chatItems)-1]
	if last.Status == "error" {
		t.Fatalf("canceled run should not mark thinking card as error")
	}
}

func TestRunCompletedSetsPhaseToIdle(t *testing.T) {
	m := model{
		async:        make(chan tea.Msg, 1),
		busy:         true,
		statusNote:   "Running...",
		phase:        "responding",
		llmConnected: true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "response", Status: "streaming"},
		},
	}

	got, _ := m.Update(runFinishedMsg{})
	updated := got.(model)

	if updated.busy {
		t.Fatalf("expected busy=false")
	}
	if updated.phase != "idle" {
		t.Fatalf("expected idle phase, got %q", updated.phase)
	}
	if updated.statusNote != "Ready." {
		t.Fatalf("expected \"Ready.\" status note, got %q", updated.statusNote)
	}
}

func TestFailRunningToolCallsMarksRunningToolsAsError(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "run command", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "content", Status: "done"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
	}
	m.failRunningToolCalls()

	running := 0
	errorCount := 0
	done := 0
	for _, item := range m.chatItems {
		if item.Kind != "tool" {
			continue
		}
		switch item.Status {
		case "running":
			running++
		case "error":
			errorCount++
		case "done":
			done++
		}
	}
	if running != 0 {
		t.Fatalf("expected 0 running tools, got %d", running)
	}
	if errorCount != 2 {
		t.Fatalf("expected 2 error tools, got %d", errorCount)
	}
	if done != 1 {
		t.Fatalf("expected 1 done tool preserved, got %d", done)
	}
}

func TestFailRunningToolCallsNoOpWhenNoRunningTools(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "content", Status: "done"},
			{Kind: "assistant", Title: assistantLabel, Body: "answer", Status: "final"},
		},
	}
	m.failRunningToolCalls()

	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done status preserved, got %q", m.chatItems[0].Status)
	}
}

func TestFailRunningToolCallsNoOpWhenEmpty(t *testing.T) {
	m := model{}
	m.failRunningToolCalls()

	if len(m.chatItems) != 0 {
		t.Fatalf("expected no change, got %d items", len(m.chatItems))
	}
}

func TestFailRunningToolRunsMarksRunningAsError(t *testing.T) {
	m := model{
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
			{Name: "read_file", Summary: "Read file.", Status: "done"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}
	m.failRunningToolRuns()

	running := 0
	errorCount := 0
	done := 0
	for _, tr := range m.toolRuns {
		switch tr.Status {
		case "running":
			running++
		case "error":
			errorCount++
		case "done":
			done++
		}
	}
	if running != 0 {
		t.Fatalf("expected 0 running, got %d", running)
	}
	if errorCount != 2 {
		t.Fatalf("expected 2 error, got %d", errorCount)
	}
	if done != 1 {
		t.Fatalf("expected 1 done preserved, got %d", done)
	}
}

func TestFailRunningToolRunsUpdatesSummary(t *testing.T) {
	m := model{
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}
	m.failRunningToolRuns()

	if m.toolRuns[0].Summary != "Tool call interrupted by error." {
		t.Fatalf("expected interrupted summary, got %q", m.toolRuns[0].Summary)
	}
}

func TestRunFailedMarksRunningToolCallsAsError(t *testing.T) {
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		statusNote:     "Running tool...",
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "run fish", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "Running...", Status: "thinking"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429")})
	updated := got.(model)

	if updated.phase != "error" {
		t.Fatalf("expected error phase, got %q", updated.phase)
	}

	toolItem := updated.chatItems[2]
	if toolItem.Status != "error" {
		t.Fatalf("expected tool card status \"error\", got %q", toolItem.Status)
	}

	if len(updated.toolRuns) != 1 {
		t.Fatalf("expected 1 tool run, got %d", len(updated.toolRuns))
	}
	if updated.toolRuns[0].Status != "error" {
		t.Fatalf("expected tool run status \"error\", got %q", updated.toolRuns[0].Status)
	}
}

func TestRunFailedWithMixedToolStates(t *testing.T) {
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		statusNote:     "Running tool...",
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "run commands", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "file content", Status: "done"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		toolRuns: []toolRun{
			{Name: "read_file", Summary: "Read file.", Status: "done"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("connection timeout")})
	updated := got.(model)

	// Done tool should remain done
	if updated.chatItems[1].Status != "done" {
		t.Fatalf("expected done tool to remain done, got %q", updated.chatItems[1].Status)
	}
	// Running tool should become error
	if updated.chatItems[2].Status != "error" {
		t.Fatalf("expected running tool to become error, got %q", updated.chatItems[2].Status)
	}
	// Thinking assistant should become error
	if updated.chatItems[3].Status != "error" {
		t.Fatalf("expected thinking card to become error, got %q", updated.chatItems[3].Status)
	}
	if updated.chatItems[3].Title != assistantLabel {
		t.Fatalf("expected error card title %q, got %q", assistantLabel, updated.chatItems[3].Title)
	}

	// Done toolRun should remain done
	if updated.toolRuns[0].Status != "done" {
		t.Fatalf("expected done toolRun to remain done, got %q", updated.toolRuns[0].Status)
	}
	// Running toolRun should become error
	if updated.toolRuns[1].Status != "error" {
		t.Fatalf("expected running toolRun to become error, got %q", updated.toolRuns[1].Status)
	}
}

// --- finalizeAssistantTurnForTool ---

func TestFinalizeAssistantTurnForToolConvertsEmptyThinkingToThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "pending"},
		},
		streamingIndex: 0,
	}
	m.finalizeAssistantTurnForTool("run_shell")

	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
	if m.chatItems[0].Status != "thinking" {
		t.Fatalf("expected thinking status, got %q", m.chatItems[0].Status)
	}
	if m.chatItems[0].Title != thinkingLabel {
		t.Fatalf("expected thinking title, got %q", m.chatItems[0].Title)
	}
}

func TestFinalizeAssistantTurnForToolRemovesGenericThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "I will call `run_shell` to inspect the relevant context first.", Status: "streaming"},
		},
		streamingIndex: 0,
	}
	m.finalizeAssistantTurnForTool("run_shell")

	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
	if len(m.chatItems) != 0 {
		t.Fatalf("expected generic thinking removed, got %d items", len(m.chatItems))
	}
}

func TestFinalizeAssistantTurnForToolKeepsMeaningfulThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "Analyzing the repository structure to understand the codebase layout", Status: "streaming"},
		},
		streamingIndex: 0,
	}
	m.finalizeAssistantTurnForTool("run_shell")

	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
	if len(m.chatItems) != 1 {
		t.Fatalf("expected meaningful thinking preserved, got %d items", len(m.chatItems))
	}
	if m.chatItems[0].Status != "thinking" {
		t.Fatalf("expected thinking status, got %q", m.chatItems[0].Status)
	}
}

func TestFinalizeAssistantTurnForToolNoOpWhenNoStreamingIndex(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "answer", Status: "final"},
		},
		streamingIndex: -1,
	}
	m.finalizeAssistantTurnForTool("run_shell")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected no change, got %d items", len(m.chatItems))
	}
}

func TestFinalizeAssistantTurnForToolNoOpWhenNotAssistant(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
		},
		streamingIndex: 0,
	}
	m.finalizeAssistantTurnForTool("run_shell")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected no change, got %d items", len(m.chatItems))
	}
}

// --- finishLatestToolCall ---

func TestFinishLatestToolCallUpdatesMatchingTool(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
	}
	m.finishLatestToolCall("run_shell", "exit code 0", "done")

	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done status, got %q", m.chatItems[0].Status)
	}
	if m.chatItems[0].Body != "exit code 0" {
		t.Fatalf("expected body update, got %q", m.chatItems[0].Body)
	}
}

func TestFinishLatestToolCallAppendsWhenNoMatch(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "content", Status: "done"},
		},
	}
	m.finishLatestToolCall("run_shell", "output", "done")

	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.chatItems))
	}
	if m.chatItems[1].Status != "done" {
		t.Fatalf("expected done status on new item, got %q", m.chatItems[1].Status)
	}
}

func TestFinishLatestToolCallUpdatesLastMatching(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
	}
	m.finishLatestToolCall("run_shell", "output", "done")

	if m.chatItems[0].Status != "running" {
		t.Fatalf("expected first to remain running, got %q", m.chatItems[0].Status)
	}
	if m.chatItems[1].Status != "done" {
		t.Fatalf("expected second to become done, got %q", m.chatItems[1].Status)
	}
}

func TestFinishLatestToolCallAppendsWhenEmpty(t *testing.T) {
	m := model{}
	m.finishLatestToolCall("run_shell", "output", "done")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.chatItems))
	}
	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done, got %q", m.chatItems[0].Status)
	}
}

// --- ensureThinkingCard ---

func TestEnsureThinkingCardCreatesNewCard(t *testing.T) {
	m := model{}
	m.ensureThinkingCard()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.chatItems))
	}
	if m.chatItems[0].Status != "pending" {
		t.Fatalf("expected pending status, got %q", m.chatItems[0].Status)
	}
	if m.streamingIndex != 0 {
		t.Fatalf("expected streamingIndex=0, got %d", m.streamingIndex)
	}
}

func TestEnsureThinkingCardReusesExisting(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "pending"},
		},
		streamingIndex: 0,
	}
	m.ensureThinkingCard()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item (reused), got %d", len(m.chatItems))
	}
	if m.chatItems[0].Status != "thinking" {
		t.Fatalf("expected thinking status after reuse, got %q", m.chatItems[0].Status)
	}
}

func TestEnsureThinkingCardCreatesNewWhenExistingIsFinal(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "answer", Status: "final"},
		},
		streamingIndex: 0,
	}
	m.ensureThinkingCard()

	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 items (new card), got %d", len(m.chatItems))
	}
	if m.chatItems[1].Status != "pending" {
		t.Fatalf("expected pending on new card, got %q", m.chatItems[1].Status)
	}
}

// --- removeStreamingAssistantPlaceholder ---

func TestRemoveStreamingAssistantPlaceholderRemovesItem(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "pending"},
		},
		streamingIndex: 1,
	}
	m.removeStreamingAssistantPlaceholder()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item after removal, got %d", len(m.chatItems))
	}
	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
}

func TestRemoveStreamingAssistantPlaceholderNoOpWhenOutOfBounds(t *testing.T) {
	m := model{
		chatItems:    []chatEntry{{Kind: "user", Title: "You", Body: "hello", Status: "final"}},
		streamingIndex: 5,
	}
	m.removeStreamingAssistantPlaceholder()

	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
	if len(m.chatItems) != 1 {
		t.Fatalf("expected no change, got %d items", len(m.chatItems))
	}
}

func TestRemoveStreamingAssistantPlaceholderNoOpWhenNotAssistant(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
		},
		streamingIndex: 0,
	}
	m.removeStreamingAssistantPlaceholder()

	if len(m.chatItems) != 1 {
		t.Fatalf("expected no change, got %d items", len(m.chatItems))
	}
	if m.streamingIndex != -1 {
		t.Fatalf("expected streamingIndex=-1, got %d", m.streamingIndex)
	}
}

// --- populateLatestThinkingToolStep ---

func TestPopulateLatestThinkingToolStepFillsEmptyCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}
	ok := m.populateLatestThinkingToolStep("run_shell", "running command", "running")

	if !ok {
		t.Fatalf("expected true")
	}
	if strings.TrimSpace(m.chatItems[0].Body) == "" {
		t.Fatalf("expected non-empty body after population, got %q", m.chatItems[0].Body)
	}
}

func TestPopulateLatestThinkingToolStepSkipsNonEmptyCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "already has content", Status: "thinking"},
		},
	}
	ok := m.populateLatestThinkingToolStep("run_shell", "running", "running")

	if ok {
		t.Fatalf("expected false for non-empty card")
	}
	if m.chatItems[0].Body != "already has content" {
		t.Fatalf("expected body unchanged, got %q", m.chatItems[0].Body)
	}
}

func TestPopulateLatestThinkingToolStepSkipsNonThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "answer", Status: "final"},
		},
	}
	ok := m.populateLatestThinkingToolStep("run_shell", "running", "running")

	if ok {
		t.Fatalf("expected false for non-thinking card")
	}
}

func TestPopulateLatestThinkingToolStepNoOpWhenEmpty(t *testing.T) {
	m := model{}
	ok := m.populateLatestThinkingToolStep("run_shell", "running", "running")

	if ok {
		t.Fatalf("expected false for empty chat")
	}
}

// --- handleAgentEvent ---

func TestHandleAgentEventDeltaAppendsToStream(t *testing.T) {
	m := model{
		phase: "thinking",
	}
	m.handleAgentEvent(Event{Type: EventAssistantDelta, Content: "Hello"})

	if m.phase != "responding" {
		t.Fatalf("expected responding phase, got %q", m.phase)
	}
	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.chatItems))
	}
	if !strings.Contains(m.chatItems[0].Body, "Hello") {
		t.Fatalf("expected delta in body, got %q", m.chatItems[0].Body)
	}
}

func TestHandleAgentEventToolCallStartedAppendsToolCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking...", Status: "streaming"},
		},
		streamingIndex: 0,
	}
	m.handleAgentEvent(Event{Type: EventToolCallStarted, ToolName: "run_shell"})

	if m.phase != "tool" {
		t.Fatalf("expected tool phase, got %q", m.phase)
	}
	found := false
	for _, item := range m.chatItems {
		if item.Kind == "tool" && item.Status == "running" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected running tool card")
	}
	if len(m.toolRuns) != 1 {
		t.Fatalf("expected 1 tool run, got %d", len(m.toolRuns))
	}
}

func TestHandleAgentEventToolCallCompletedUpdatesToolCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "run_shell",
		ToolResult: `{"ok":true,"exit_code":0,"stdout":"hello","stderr":""}`,
	})

	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done status, got %q", m.chatItems[0].Status)
	}
	if m.toolRuns[0].Status != "done" {
		t.Fatalf("expected tool run done, got %q", m.toolRuns[0].Status)
	}
	if m.phase != "thinking" {
		t.Fatalf("expected thinking phase after tool completion, got %q", m.phase)
	}
}

func TestHandleAgentEventRunFinishedSetsIdle(t *testing.T) {
	m := model{
		phase: "responding",
	}
	m.handleAgentEvent(Event{Type: EventRunFinished, Content: "done"})

	if m.phase != "idle" {
		t.Fatalf("expected idle phase, got %q", m.phase)
	}
}

func TestHandleAgentEventRunStartedResetsEstimatedOutput(t *testing.T) {
	m := model{
		tempEstimatedOutput: 500,
	}
	m.handleAgentEvent(Event{Type: EventRunStarted})

	if m.tempEstimatedOutput != 0 {
		t.Fatalf("expected 0, got %d", m.tempEstimatedOutput)
	}
}

func TestHandleAgentEventAssistantDeltaAppliesEstimatedUsageWithoutOfficialUsage(t *testing.T) {
	m := model{
		tokenUsage:     newTokenUsageComponent(),
		tokenBudget:    5000,
		tokenEstimator: newRealtimeTokenEstimator(""),
	}

	m.handleAgentEvent(Event{Type: EventAssistantDelta, Content: "hello world from stream"})

	if m.tempEstimatedOutput <= 0 {
		t.Fatalf("expected estimated output tokens > 0, got %d", m.tempEstimatedOutput)
	}
	if m.tokenUsedTotal != m.tempEstimatedOutput {
		t.Fatalf("expected used tokens to match estimate, got used=%d estimate=%d", m.tokenUsedTotal, m.tempEstimatedOutput)
	}
	if m.tokenOutput != m.tempEstimatedOutput {
		t.Fatalf("expected output tokens to match estimate, got output=%d estimate=%d", m.tokenOutput, m.tempEstimatedOutput)
	}
	if m.tokenUsage.used != m.tempEstimatedOutput {
		t.Fatalf("expected token monitor to show estimate, got %d", m.tokenUsage.used)
	}
	if m.tokenUsage.unavailable {
		t.Fatal("expected token monitor to become available after estimate")
	}
}

func TestHandleAgentEventAssistantDeltaDoesNotDoubleCountAfterOfficialUsage(t *testing.T) {
	m := model{
		tokenUsage:     newTokenUsageComponent(),
		tokenBudget:    5000,
		tokenEstimator: newRealtimeTokenEstimator(""),
	}

	m.handleAgentEvent(Event{Type: EventAssistantDelta, Content: "hello world from stream"})
	estimated := m.tempEstimatedOutput
	if estimated <= 0 {
		t.Fatalf("expected estimate > 0, got %d", estimated)
	}

	m.handleAgentEvent(Event{Type: EventUsageUpdated, Usage: llm.Usage{InputTokens: 20, OutputTokens: 7, ContextTokens: 3, TotalTokens: 30}})
	m.handleAgentEvent(Event{Type: EventAssistantDelta, Content: "more trailing streamed text"})

	if m.tempEstimatedOutput != 0 {
		t.Fatalf("expected estimate to be cleared by official usage, got %d", m.tempEstimatedOutput)
	}
	if m.tokenUsedTotal != 30 {
		t.Fatalf("expected official total 30 without double counting, got %d", m.tokenUsedTotal)
	}
	if m.tokenOutput != 7 {
		t.Fatalf("expected official output 7 without double counting, got %d", m.tokenOutput)
	}
}

func TestHandleAgentEventPlanUpdatedCopiesPlan(t *testing.T) {
	m := model{
		phase: "thinking",
	}
	m.handleAgentEvent(Event{
		Type: EventPlanUpdated,
		Plan: planpkg.State{
			Goal:  "Build feature",
			Phase: planpkg.PhaseReady,
			Steps: []planpkg.Step{{Title: "Step 1", Status: planpkg.StepPending}},
		},
	})

	if m.plan.Goal != "Build feature" {
		t.Fatalf("expected plan goal, got %q", m.plan.Goal)
	}
	if len(m.plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(m.plan.Steps))
	}
}

// --- Rate limit error scenarios (from screenshot) ---

func TestRunFailedWithToolResultErrorContent(t *testing.T) {
	// Scenario: tool result card has error body from rate limit, then run fails
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		statusNote:     "Running tool...",
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "search code", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("search_text"), Body: "Request failed: provider rate limited: 429 You have requ", Status: "error"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		toolRuns: []toolRun{
			{Name: "search_text", Summary: "Request failed: provider rate limited", Status: "error"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429")})
	updated := got.(model)

	// Thinking card (last item) should become error with correct title
	last := updated.chatItems[len(updated.chatItems)-1]
	if last.Status != "error" {
		t.Fatalf("expected error status on last card, got %q", last.Status)
	}
	if last.Title != assistantLabel {
		t.Fatalf("expected assistant label on error card, got %q", last.Title)
	}

	// Tool card should remain error (not changed to running)
	toolCard := updated.chatItems[1]
	if toolCard.Status != "error" {
		t.Fatalf("expected tool card to remain error, got %q", toolCard.Status)
	}
}

func TestRunFailedAfterMultipleToolCalls(t *testing.T) {
	// Scenario: multiple tools executed, last one running when error hits
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		statusNote:     "Running tool...",
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "do stuff", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "Read 5 files", Status: "done"},
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read main.go", Status: "done"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		toolRuns: []toolRun{
			{Name: "list_files", Summary: "Listed files.", Status: "done"},
			{Name: "read_file", Summary: "Read file.", Status: "done"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited")})
	updated := got.(model)

	// Done tools should remain done
	if updated.chatItems[1].Status != "done" {
		t.Fatalf("expected list_files to remain done, got %q", updated.chatItems[1].Status)
	}
	if updated.chatItems[2].Status != "done" {
		t.Fatalf("expected read_file to remain done, got %q", updated.chatItems[2].Status)
	}
	// Running tool should become error
	if updated.chatItems[3].Status != "error" {
		t.Fatalf("expected run_shell to become error, got %q", updated.chatItems[3].Status)
	}
	// Thinking card should become error
	if updated.chatItems[4].Status != "error" {
		t.Fatalf("expected thinking card to become error, got %q", updated.chatItems[4].Status)
	}
	if updated.chatItems[4].Title != assistantLabel {
		t.Fatalf("expected error card title %q, got %q", assistantLabel, updated.chatItems[4].Title)
	}
	// Done toolRuns should remain done
	if updated.toolRuns[0].Status != "done" {
		t.Fatalf("expected list_files toolRun done, got %q", updated.toolRuns[0].Status)
	}
	// Running toolRun should become error
	if updated.toolRuns[2].Status != "error" {
		t.Fatalf("expected run_shell toolRun error, got %q", updated.toolRuns[2].Status)
	}
}

func TestRunFailedSetsIndicatorState(t *testing.T) {
	m := model{
		async:       make(chan tea.Msg, 1),
		busy:        true,
		activeRunID: 5,
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}

	got, _ := m.Update(runFinishedMsg{RunID: 5, Err: errors.New("rate limit")})
	updated := got.(model)

	if updated.runIndicatorState != runIndicatorFailed {
		t.Fatalf("expected runIndicatorFailed, got %q", updated.runIndicatorState)
	}
	if updated.activeRunID != 0 {
		t.Fatalf("expected activeRunID=0, got %d", updated.activeRunID)
	}
	if updated.runCancel != nil {
		t.Fatalf("expected runCancel=nil")
	}
}

func TestRunFailedPhaseError(t *testing.T) {
	m := model{
		async:       make(chan tea.Msg, 1),
		busy:        true,
		activeRunID: 3,
		phase:       "tool",
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
	}

	got, _ := m.Update(runFinishedMsg{RunID: 3, Err: errors.New("429 Too Many Requests")})
	updated := got.(model)

	if updated.phase != "error" {
		t.Fatalf("expected error phase, got %q", updated.phase)
	}
	if !strings.Contains(updated.statusNote, "429") {
		t.Fatalf("expected 429 in status note, got %q", updated.statusNote)
	}
	if updated.llmConnected {
		t.Fatalf("expected llmConnected=false")
	}
}

func TestHandleAgentEventToolCallCompletedWithRateLimitError(t *testing.T) {
	// When tool result contains rate limit error JSON
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "run_shell",
		ToolResult: `{"ok":false,"error":"provider rate limited: 429"}`,
	})

	if m.chatItems[0].Status != "error" {
		t.Fatalf("expected error status, got %q", m.chatItems[0].Status)
	}
	if m.toolRuns[0].Status != "error" {
		t.Fatalf("expected tool run error, got %q", m.toolRuns[0].Status)
	}
	if m.phase != "thinking" {
		t.Fatalf("expected thinking phase, got %q", m.phase)
	}
}

// --- Screenshot scenario: tool result with error text + empty running card ---

func TestToolResultCardShowsOnlyToolResult(t *testing.T) {
	// A completed tool result should show clean content, not error text
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "read_file", Summary: "Tool call started.", Status: "running"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"/project/main.go","start_line":1,"end_line":50}`,
	})

	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done, got %q", m.chatItems[0].Status)
	}
	if strings.Contains(m.chatItems[0].Body, "Request failed") {
		t.Fatalf("tool result should not contain error text, got %q", m.chatItems[0].Body)
	}
	if !strings.Contains(m.chatItems[0].Body, "main.go") {
		t.Fatalf("expected file name in body, got %q", m.chatItems[0].Body)
	}
}

func TestRunFailedEmptyRunningToolCardBecomesError(t *testing.T) {
	// An empty tool card in "running" state should become "error" on run failure
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "do something", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429")})
	updated := got.(model)

	if updated.chatItems[1].Status != "error" {
		t.Fatalf("expected empty running tool card to become error, got %q", updated.chatItems[1].Status)
	}
	if updated.toolRuns[0].Status != "error" {
		t.Fatalf("expected tool run to become error, got %q", updated.toolRuns[0].Status)
	}
}

func TestRunFailedCompletedToolThenRunningTool(t *testing.T) {
	// Scenario: read_file completed, then run_shell started but run failed
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "read and run", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read main.go\nrange: 1-50", Status: "done"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		toolRuns: []toolRun{
			{Name: "read_file", Summary: "Read main.go", Status: "done"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429 You have requ")})
	updated := got.(model)

	// Completed tool should remain done
	if updated.chatItems[1].Status != "done" {
		t.Fatalf("expected read_file to remain done, got %q", updated.chatItems[1].Status)
	}
	if strings.Contains(updated.chatItems[1].Body, "Request failed") {
		t.Fatalf("completed tool body should not be modified, got %q", updated.chatItems[1].Body)
	}

	// Running tool should become error
	if updated.chatItems[2].Status != "error" {
		t.Fatalf("expected run_shell to become error, got %q", updated.chatItems[2].Status)
	}

	// Thinking card should become error
	if updated.chatItems[3].Status != "error" {
		t.Fatalf("expected thinking card to become error, got %q", updated.chatItems[3].Status)
	}
	if updated.chatItems[3].Title != assistantLabel {
		t.Fatalf("expected assistant label, got %q", updated.chatItems[3].Title)
	}

	// Done toolRun should remain done
	if updated.toolRuns[0].Status != "done" {
		t.Fatalf("expected read_file toolRun done, got %q", updated.toolRuns[0].Status)
	}
	// Running toolRun should become error
	if updated.toolRuns[1].Status != "error" {
		t.Fatalf("expected run_shell toolRun error, got %q", updated.toolRuns[1].Status)
	}
}

func TestRunFailedToolResultWithEmbeddedErrorText(t *testing.T) {
	// Scenario: tool result body contains error text (e.g., from provider)
	// The card should show the error status correctly
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "read file", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read main.go\nrange: 1-50\npath: /project/main.go\nRequest failed: provider rate limited: 429 You have requ", Status: "error"},
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		toolRuns: []toolRun{
			{Name: "read_file", Summary: "Read main.go", Status: "error"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429")})
	updated := got.(model)

	// Thinking card should become error
	if updated.chatItems[2].Status != "error" {
		t.Fatalf("expected thinking card error, got %q", updated.chatItems[2].Status)
	}
	if updated.chatItems[2].Title != assistantLabel {
		t.Fatalf("expected assistant label, got %q", updated.chatItems[2].Title)
	}

	// Tool card should remain error (not modified further)
	if updated.chatItems[1].Status != "error" {
		t.Fatalf("expected tool card to remain error, got %q", updated.chatItems[1].Status)
	}
}

func TestMultipleRunningToolCardsAllBecomeError(t *testing.T) {
	// Scenario: multiple tools started simultaneously, all should become error
	m := model{
		async:          make(chan tea.Msg, 1),
		busy:           true,
		streamingIndex: -1,
		phase:          "tool",
		llmConnected:   true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "run commands", Status: "final"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
		toolRuns: []toolRun{
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
			{Name: "run_shell", Summary: "Tool call started.", Status: "running"},
		},
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited")})
	updated := got.(model)

	for i, item := range updated.chatItems {
		if item.Kind == "tool" && item.Status != "error" {
			t.Fatalf("expected tool card %d to be error, got %q", i, item.Status)
		}
	}
	for i, tr := range updated.toolRuns {
		if tr.Status != "error" {
			t.Fatalf("expected tool run %d to be error, got %q", i, tr.Status)
		}
	}
}

// --- formatElapsedWords ---

func TestFormatElapsedWordsZeroStart(t *testing.T) {
	got := formatElapsedWords(time.Time{}, time.Now())
	if got != "0s" {
		t.Fatalf("expected \"0s\" for zero start, got %q", got)
	}
}

func TestFormatElapsedWordsNegativeTime(t *testing.T) {
	now := time.Now()
	got := formatElapsedWords(now.Add(10*time.Second), now)
	if got != "0s" {
		t.Fatalf("expected \"0s\" for negative elapsed, got %q", got)
	}
}

func TestFormatElapsedWordsSecondsOnly(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(45 * time.Second)
	got := formatElapsedWords(start, end)
	if got != "45s" {
		t.Fatalf("expected \"45s\", got %q", got)
	}
}

func TestFormatElapsedWordsMinutesAndSeconds(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2*time.Minute + 30*time.Second)
	got := formatElapsedWords(start, end)
	if got != "2m 30s" {
		t.Fatalf("expected \"2m 30s\", got %q", got)
	}
}

func TestFormatElapsedWordsHoursMinutesSeconds(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(1*time.Hour + 5*time.Minute + 10*time.Second)
	got := formatElapsedWords(start, end)
	if got != "1h 5m 10s" {
		t.Fatalf("expected \"1h 5m 10s\", got %q", got)
	}
}

func TestFormatElapsedWordsExactlyOneMinute(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(60 * time.Second)
	got := formatElapsedWords(start, end)
	if got != "1m" {
		t.Fatalf("expected \"1m\", got %q", got)
	}
}

func TestFormatElapsedWordsExactlyOneHour(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(3600 * time.Second)
	got := formatElapsedWords(start, end)
	if got != "1h" {
		t.Fatalf("expected \"1h\", got %q", got)
	}
}

// --- decorateFinalAnswer ---

func TestDecorateFinalAnswerEmptyContent(t *testing.T) {
	m := model{runStartedAt: time.Now()}
	if got := m.decorateFinalAnswer(""); got != "" {
		t.Fatalf("expected empty for empty content, got %q", got)
	}
	if got := m.decorateFinalAnswer("   "); got != "" {
		t.Fatalf("expected empty for whitespace content, got %q", got)
	}
}

func TestDecorateFinalAnswerZeroStartedAt(t *testing.T) {
	m := model{}
	got := m.decorateFinalAnswer("hello")
	if got != "hello" {
		t.Fatalf("expected unchanged content for zero startedAt, got %q", got)
	}
}

func TestDecorateFinalAnswerAlreadyDecorated(t *testing.T) {
	m := model{runStartedAt: time.Now()}
	content := "answer\n\nProcessed for 5s"
	got := m.decorateFinalAnswer(content)
	if got != content {
		t.Fatalf("expected unchanged for already decorated content, got %q", got)
	}
}

func TestDecorateFinalAnswerAlreadyDecoratedCompleted(t *testing.T) {
	m := model{runStartedAt: time.Now()}
	content := "answer\n\nCompleted in 3s"
	got := m.decorateFinalAnswer(content)
	if got != content {
		t.Fatalf("expected unchanged for completed content, got %q", got)
	}
}

func TestDecorateFinalAnswerAddsElapsed(t *testing.T) {
	m := model{runStartedAt: time.Now().Add(-5 * time.Second)}
	got := m.decorateFinalAnswer("the answer")
	if !strings.Contains(got, "the answer") {
		t.Fatalf("expected original content preserved, got %q", got)
	}
	if !strings.Contains(got, "Processed for") {
		t.Fatalf("expected elapsed decoration, got %q", got)
	}
}

// --- appendAssistantToolFollowUp ---

func TestAppendAssistantToolFollowUpEmptySummary(t *testing.T) {
	m := model{}
	m.chatItems = []chatEntry{{Kind: "assistant", Body: "existing"}}
	m.appendAssistantToolFollowUp("run_shell", "", "done")
	// Empty summary still generates a default follow-up message
	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 items (default follow-up), got %d", len(m.chatItems))
	}
}

func TestAppendAssistantToolFollowUpNormalAppend(t *testing.T) {
	m := model{}
	m.chatItems = []chatEntry{{Kind: "assistant", Body: "existing"}}
	m.appendAssistantToolFollowUp("run_shell", "exit code 0", "done")
	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.chatItems))
	}
	if m.chatItems[1].Body == "" {
		t.Fatalf("expected non-empty follow-up body")
	}
}

func TestAppendAssistantToolFollowUpErrorStatus(t *testing.T) {
	m := model{}
	m.chatItems = []chatEntry{{Kind: "assistant", Body: "existing"}}
	m.appendAssistantToolFollowUp("run_shell", "command not found", "error")
	if len(m.chatItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.chatItems))
	}
}

func TestAppendAssistantToolFollowUpEmptyChatItems(t *testing.T) {
	m := model{}
	m.appendAssistantToolFollowUp("read_file", "read file.go", "done")
	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.chatItems))
	}
}
