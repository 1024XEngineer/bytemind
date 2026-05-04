package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
