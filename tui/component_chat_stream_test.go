package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
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
}

// --- finalizeAssistantTurnForTool ---

func TestFinalizeAssistantTurnForToolRemovesEmptyThinkingCard(t *testing.T) {
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
	if len(m.chatItems) != 0 {
		t.Fatalf("expected empty thinking card removed, got %d items", len(m.chatItems))
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

func TestFinalizeAssistantTurnForToolRemovesMeaningfulThinking(t *testing.T) {
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
	if len(m.chatItems) != 0 {
		t.Fatalf("expected meaningful thinking removed from chat timeline, got %d items", len(m.chatItems))
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
		chatItems:      []chatEntry{{Kind: "user", Title: "You", Body: "hello", Status: "final"}},
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
}

func TestHandleAgentEventToolCallStartedCapturesCompactHint(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "", Status: "thinking"},
		},
		streamingIndex: 0,
	}

	m.handleAgentEvent(Event{
		Type:          EventToolCallStarted,
		ToolName:      "search_text",
		ToolCallID:    "call-hint-1",
		ToolArguments: `{"query":"isMeaningfulThinking","path":"tui/model.go"}`,
	})
	if len(m.chatItems) == 0 {
		t.Fatal("expected tool item to be appended")
	}
	last := m.chatItems[len(m.chatItems)-1]
	if last.Kind != "tool" || last.ToolCallID != "call-hint-1" {
		t.Fatalf("expected latest chat item to be started tool call, got %+v", last)
	}
	if last.CompactBody != `"isMeaningfulThinking"` {
		t.Fatalf("expected tool compact hint from arguments, got %q", last.CompactBody)
	}
}

func TestHandleAgentEventToolCallCompletedUpdatesToolCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running", ToolCallID: "call-1"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "run_shell",
		ToolCallID: "call-1",
		ToolResult: `{"ok":true,"exit_code":0,"stdout":"hello","stderr":""}`,
	})

	if m.chatItems[0].Status != "done" {
		t.Fatalf("expected done status, got %q", m.chatItems[0].Status)
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

func TestHandleAgentEventThinkingProgressCreatesThinkingCard(t *testing.T) {
	m := model{
		phase:  "thinking",
		spinner: spinner.New(),
	}
	m.handleAgentEvent(Event{
		Type:               EventThinkingProgress,
		ReasoningCharCount: 150,
		ReasoningActive:    true,
	})

	if !m.reasoningProgressActive {
		t.Fatal("expected reasoningProgressActive to be true after progress event")
	}
	if m.phase != "thinking" {
		t.Fatalf("expected thinking phase, got %q", m.phase)
	}
	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item (thinking card), got %d", len(m.chatItems))
	}
	item := m.chatItems[0]
	if item.Kind != "assistant" {
		t.Fatalf("expected assistant kind, got %q", item.Kind)
	}
	if item.Status != "thinking" {
		t.Fatalf("expected thinking status, got %q", item.Status)
	}
	if !strings.Contains(item.Body, "receiving hidden reasoning") {
		t.Fatalf("expected reasoning progress note in body, got %q", item.Body)
	}
	// Reasoning content must NOT appear in the body.
	if strings.Contains(item.Body, "step") {
		t.Fatalf("body must not contain reasoning content, got %q", item.Body)
	}
}

func TestHandleAgentEventThinkingProgressInactiveRemovesNote(t *testing.T) {
	m := model{
		phase:                  "thinking",
		reasoningProgressActive: true,
		spinner:                 spinner.New(),
	}
	m.handleAgentEvent(Event{
		Type:               EventThinkingProgress,
		ReasoningCharCount: 300,
		ReasoningActive:    false,
	})

	if m.reasoningProgressActive {
		t.Fatal("expected reasoningProgressActive to be false after inactive event")
	}
	if len(m.chatItems) != 1 {
		t.Fatalf("expected thinking card, got %d items", len(m.chatItems))
	}
	if strings.Contains(m.chatItems[0].Body, "receiving hidden reasoning") {
		t.Fatalf("expected reasoning note removed, got %q", m.chatItems[0].Body)
	}
}

func TestHandleAgentEventAssistantDeltaClearsReasoningProgress(t *testing.T) {
	m := model{
		phase:                  "thinking",
		reasoningProgressActive: true,
	}
	m.handleAgentEvent(Event{Type: EventAssistantDelta, Content: "Here is the answer."})

	if m.reasoningProgressActive {
		t.Fatal("expected reasoningProgressActive to be cleared by assistant delta")
	}
	if m.phase != "responding" {
		t.Fatalf("expected responding phase, got %q", m.phase)
	}
}

func TestHandleAgentEventToolCallStartedClearsReasoningProgress(t *testing.T) {
	m := model{
		phase:                  "thinking",
		reasoningProgressActive: true,
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking...", Status: "thinking"},
		},
		streamingIndex: 0,
	}
	m.handleAgentEvent(Event{Type: EventToolCallStarted, ToolName: "list_files"})

	if m.reasoningProgressActive {
		t.Fatal("expected reasoningProgressActive to be cleared by tool call start")
	}
}

func TestUpdateThinkingCardPreservesReasoningNote(t *testing.T) {
	m := model{
		busy:                    true,
		streamingIndex:          0,
		reasoningProgressActive: true,
		chatItems: []chatEntry{
			{Kind: "assistant", Title: thinkingLabel, Body: "old body", Status: "thinking"},
		},
	}
	m.updateThinkingCard()

	if m.chatItems[0].Body != "receiving hidden reasoning..." {
		t.Fatalf("expected reasoning note to be preserved, got %q", m.chatItems[0].Body)
	}
}

func TestApplyReasoningProgressSkipsWhenItemIsNotThinkingAssistant(t *testing.T) {
	m := model{
		streamingIndex: 0,
		chatItems: []chatEntry{
			{Kind: "tool", Title: "TOOL | list_files", Body: "done", Status: "done"},
		},
	}
	m.applyReasoningProgress(100, true)
	if m.chatItems[0].Body != "done" {
		t.Fatalf("expected tool item to be untouched, got %q", m.chatItems[0].Body)
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
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running", ToolCallID: "call-2"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "run_shell",
		ToolCallID: "call-2",
		ToolResult: `{"ok":false,"error":"provider rate limited: 429"}`,
	})

	if m.chatItems[0].Status != "error" {
		t.Fatalf("expected error status, got %q", m.chatItems[0].Status)
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
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "", Status: "running", ToolCallID: "call-3"},
		},
	}
	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "read_file",
		ToolCallID: "call-3",
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
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited: 429")})
	updated := got.(model)

	if updated.chatItems[1].Status != "error" {
		t.Fatalf("expected empty running tool card to become error, got %q", updated.chatItems[1].Status)
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
	}

	got, _ := m.Update(runFinishedMsg{Err: errors.New("provider rate limited")})
	updated := got.(model)

	for i, item := range updated.chatItems {
		if item.Kind == "tool" && item.Status != "error" {
			t.Fatalf("expected tool card %d to be error, got %q", i, item.Status)
		}
	}
}

func TestToolCallStartedMarksPreviousRunningAsQueued(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "", Status: "running"},
		},
	}
	m.handleAgentEvent(Event{Type: EventToolCallStarted, ToolName: "run_shell"})

	if m.chatItems[0].Status != "queued" {
		t.Fatalf("expected previous running tool to become queued, got %q", m.chatItems[0].Status)
	}
	found := false
	for _, item := range m.chatItems {
		if item.Kind == "tool" && item.Status == "running" && item.Title == toolEntryTitle("run_shell") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected new running tool card for run_shell")
	}
}

func TestFailRunningToolCallsAlsoMarksQueuedAsError(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "", Status: "queued"},
			{Kind: "tool", Title: toolEntryTitle("run_shell"), Body: "", Status: "running"},
		},
	}
	m.failRunningToolCalls()

	for _, item := range m.chatItems {
		if item.Status != "error" {
			t.Fatalf("expected all queued/running tools to become error, got %q for %s", item.Status, item.Title)
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

// --- stall detection ---

func TestStallDetectionSetsStalledAfterTimeout(t *testing.T) {
	m := model{busy: true}
	m.spinner = newThinkingSpinner()
	m.lastTokenReceivedAt = time.Now().Add(-5 * time.Second)

	result, _ := m.Update(spinner.TickMsg{})
	got := result.(model)
	if !got.stalled {
		t.Fatal("expected stalled=true after >3s without activity")
	}
}

func TestStallDetectionClearsOnActivity(t *testing.T) {
	m := model{busy: true, stalled: true}
	m.spinner = newThinkingSpinner()
	m.lastTokenReceivedAt = time.Now()

	result, _ := m.Update(spinner.TickMsg{})
	got := result.(model)
	if got.stalled {
		t.Fatal("expected stalled=false after fresh activity")
	}
}

func TestStallDetectionSkipsWhenNotBusy(t *testing.T) {
	m := model{busy: false}
	m.spinner = newThinkingSpinner()
	m.lastTokenReceivedAt = time.Now().Add(-10 * time.Second)

	result, _ := m.Update(spinner.TickMsg{})
	got := result.(model)
	if got.stalled {
		t.Fatal("expected stalled=false when not busy")
	}
}

func TestHandleAgentEventDelegateSubagentParsesTaskAndSetsTotalToolCalls(t *testing.T) {
	cancelCalls := 0
	m := model{
		runCancel: func() {
			cancelCalls++
		},
		interruptSafe:    true,
		interrupting:     true,
		pendingInterrupt: true,
	}

	m.handleAgentEvent(Event{
		Type:          EventToolCallStarted,
		ToolName:      "delegate_subagent",
		ToolCallID:    "delegate-1",
		ToolArguments: `{"agent":"reviewer","task":"scan login flow"}`,
	})

	if len(m.chatItems) == 0 {
		t.Fatal("expected delegate_subagent start to append a tool entry")
	}
	entry := m.chatItems[len(m.chatItems)-1]
	if entry.AgentID != "reviewer" {
		t.Fatalf("expected AgentID=reviewer, got %q", entry.AgentID)
	}
	if entry.TaskPrompt != "scan login flow" {
		t.Fatalf("expected TaskPrompt from arguments, got %q", entry.TaskPrompt)
	}
	if entry.CompactBody != "scan login flow" {
		t.Fatalf("expected CompactBody to use task prompt, got %q", entry.CompactBody)
	}

	m.chatItems[len(m.chatItems)-1].SubAgentTools = []SubAgentToolCall{
		{ToolName: "read_file", Status: "done"},
		{ToolName: "search_text", Status: "done"},
	}

	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		ToolName:   "delegate_subagent",
		ToolCallID: "delegate-1",
		ToolResult: `{"ok":true,"agent":"reviewer","summary":"finished"}`,
	})

	final := m.chatItems[len(m.chatItems)-1]
	if final.TotalToolCalls != 2 {
		t.Fatalf("expected TotalToolCalls to be set from subagent tools, got %d", final.TotalToolCalls)
	}
	if cancelCalls != 1 {
		t.Fatalf("expected interrupt-safe completion to invoke runCancel once, got %d", cancelCalls)
	}
	if m.pendingInterrupt {
		t.Fatal("expected pending interrupt flag to be cleared after safe completion")
	}
	if m.phase != "interrupting" {
		t.Fatalf("expected phase to stay interrupting while canceling run, got %q", m.phase)
	}
}

func TestHandleAgentEventDelegateSubagentMissingAgentFallsBackToDefault(t *testing.T) {
	m := model{}
	m.handleAgentEvent(Event{
		Type:          EventToolCallStarted,
		ToolName:      "delegate_subagent",
		ToolArguments: `{"task":"triage flaky tests"}`,
	})
	if len(m.chatItems) == 0 {
		t.Fatal("expected tool entry to be appended")
	}
	last := m.chatItems[len(m.chatItems)-1]
	if last.AgentID != "subagent" {
		t.Fatalf("expected missing agent to fall back to subagent, got %q", last.AgentID)
	}
	if last.TaskPrompt != "triage flaky tests" {
		t.Fatalf("expected task prompt to be captured, got %q", last.TaskPrompt)
	}
}

func TestHandleSubAgentEventRoutesByAgentIDAndFallbacks(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{
				Kind:    "tool",
				Title:   toolEntryTitle("delegate_subagent"),
				Status:  "running",
				AgentID: "explorer",
			},
			{
				Kind:    "tool",
				Title:   toolEntryTitle("delegate_subagent"),
				Status:  "running",
				AgentID: "reviewer",
			},
		},
	}

	m.handleAgentEvent(Event{
		Type:          EventToolCallStarted,
		AgentID:       "reviewer",
		ToolName:      "read_file",
		ToolCallID:    "sub-1",
		ToolArguments: `{"path":"pkg/auth.go","start_line":1,"end_line":20}`,
	})

	if len(m.chatItems[0].SubAgentTools) != 0 {
		t.Fatalf("expected explorer entry untouched, got %#v", m.chatItems[0].SubAgentTools)
	}
	if len(m.chatItems[1].SubAgentTools) != 1 {
		t.Fatalf("expected reviewer entry to receive one tool call, got %#v", m.chatItems[1].SubAgentTools)
	}
	if m.chatItems[1].SubAgentTools[0].ToolCallID != "sub-1" {
		t.Fatalf("expected precise ToolCallID routing, got %#v", m.chatItems[1].SubAgentTools[0])
	}

	m.handleAgentEvent(Event{
		Type:       EventToolCallCompleted,
		AgentID:    "reviewer",
		ToolName:   "read_file",
		ToolCallID: "sub-1",
		ToolResult: `{"path":"pkg/auth.go","start_line":1,"end_line":20}`,
	})
	if m.chatItems[1].SubAgentTools[0].Status == "running" {
		t.Fatalf("expected ToolCallID completion path to update status, got %#v", m.chatItems[1].SubAgentTools[0])
	}

	m.chatItems[1].SubAgentTools = append(m.chatItems[1].SubAgentTools, SubAgentToolCall{
		ToolName: "search_text",
		Status:   "running",
	})
	m.handleSubAgentEvent(Event{
		Type:       EventToolCallCompleted,
		AgentID:    "unknown-agent",
		ToolName:   "search_text",
		ToolResult: `{"query":"token","matches":[{"path":"a.go","line":8,"text":"token"}]}`,
	})
	if got := m.chatItems[1].SubAgentTools[1].Status; got == "running" {
		t.Fatalf("expected fallback completion by name to update status, got %q", got)
	}

	if got := m.findActiveSubAgentEntryByID(" reviewer "); got == nil {
		t.Fatal("expected trimmed AgentID to resolve active subagent entry")
	}
	m.chatItems[1].Status = "done"
	if got := m.findActiveSubAgentEntryByID("reviewer"); got != nil {
		t.Fatalf("expected non-running entries to be excluded, got %#v", *got)
	}
}
