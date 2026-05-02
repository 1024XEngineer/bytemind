package tui

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestApplyUsageEarlyReturnWhenPayloadHasNoTokens(t *testing.T) {
	m := model{
		tokenUsage:            newTokenUsageComponent(),
		tokenHasOfficialUsage: false,
		tokenUsedTotal:        77,
		tokenInput:            30,
		tokenOutput:           40,
		tokenContext:          7,
		tempEstimatedOutput:   12,
	}

	m.applyUsage(llm.Usage{})

	if !m.tokenHasOfficialUsage {
		t.Fatal("expected applyUsage to mark official usage path")
	}
	if m.tokenUsedTotal != 77 || m.tokenInput != 30 || m.tokenOutput != 40 || m.tokenContext != 7 {
		t.Fatalf("expected zero-payload early return to keep counters unchanged, got used=%d input=%d output=%d context=%d", m.tokenUsedTotal, m.tokenInput, m.tokenOutput, m.tokenContext)
	}
	if m.tempEstimatedOutput != 12 {
		t.Fatalf("expected early return not to touch temporary estimate, got %d", m.tempEstimatedOutput)
	}
}

func TestApplyUsageReplacesTemporaryEstimateBeforeAccumulating(t *testing.T) {
	m := model{
		tokenUsage:          newTokenUsageComponent(),
		tokenUsedTotal:      100,
		tokenInput:          20,
		tokenOutput:         60,
		tokenContext:        20,
		tempEstimatedOutput: 15,
	}

	m.applyUsage(llm.Usage{
		InputTokens:   10,
		OutputTokens:  5,
		ContextTokens: 2,
		TotalTokens:   20,
	})

	if m.tempEstimatedOutput != 0 {
		t.Fatalf("expected temporary estimate to be reset, got %d", m.tempEstimatedOutput)
	}
	if m.tokenUsedTotal != 105 || m.tokenInput != 30 || m.tokenOutput != 50 || m.tokenContext != 22 {
		t.Fatalf("unexpected counters after replacing estimate and applying official usage: used=%d input=%d output=%d context=%d", m.tokenUsedTotal, m.tokenInput, m.tokenOutput, m.tokenContext)
	}
	if m.tokenUsage.unavailable {
		t.Fatal("expected token monitor to be marked available after official usage update")
	}
}

func TestSetUsageMarksOfficialAndAvailable(t *testing.T) {
	m := model{
		tokenUsage: newTokenUsageComponent(),
	}
	m.tokenUsage.SetUnavailable(true)

	_ = m.SetUsage(123, 5000)

	if !m.tokenHasOfficialUsage {
		t.Fatal("expected SetUsage to mark official usage flag")
	}
	if m.tokenUsage.unavailable {
		t.Fatal("expected SetUsage to clear unavailable state")
	}
	if m.tokenUsage.used != 123 {
		t.Fatalf("expected SetUsage to update used tokens, got %d", m.tokenUsage.used)
	}
}

func TestRenderStartupGuidePanelDefaultsAndLineFiltering(t *testing.T) {
	m := model{
		width: 100,
		startupGuide: StartupGuide{
			Lines:        []string{" first line ", "   ", "second line"},
			CurrentField: "",
		},
	}

	view := m.renderStartupGuidePanel()
	if !strings.Contains(view, "Provider setup required") {
		t.Fatalf("expected default startup title, got %q", view)
	}
	if !strings.Contains(view, "AI provider is not available.") {
		t.Fatalf("expected default startup status, got %q", view)
	}
	if !strings.Contains(view, "first line") || !strings.Contains(view, "second line") {
		t.Fatalf("expected non-empty startup lines to render, got %q", view)
	}
	if !strings.Contains(view, "Input value then press Enter.") {
		t.Fatalf("expected fallback input hint for unknown field, got %q", view)
	}
}

func TestRestoreTokenUsageFromSessionNilResetsCounters(t *testing.T) {
	m := model{
		tokenUsedTotal:        88,
		tokenInput:            40,
		tokenOutput:           33,
		tokenContext:          15,
		tokenHasOfficialUsage: true,
		tempEstimatedOutput:   10,
	}

	m.restoreTokenUsageFromSession(nil)

	if m.tokenHasOfficialUsage {
		t.Fatal("expected nil-session restore to clear official-usage flag")
	}
	if m.tokenUsedTotal != 0 || m.tokenInput != 0 || m.tokenOutput != 0 || m.tokenContext != 0 {
		t.Fatalf("expected nil-session restore to zero counters, got used=%d input=%d output=%d context=%d", m.tokenUsedTotal, m.tokenInput, m.tokenOutput, m.tokenContext)
	}
	if m.tempEstimatedOutput != 0 {
		t.Fatalf("expected nil-session restore to clear temporary estimate, got %d", m.tempEstimatedOutput)
	}
}
