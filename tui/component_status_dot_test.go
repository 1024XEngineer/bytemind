package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusDotTickCommandsEmitExpectedMsgs(t *testing.T) {
	tickCmd := statusDotTickCmd()
	if tickCmd == nil {
		t.Fatal("expected status dot tick command")
	}
	if _, ok := tickCmd().(statusDotTickMsg); !ok {
		t.Fatalf("expected statusDotTickMsg, got %T", tickCmd())
	}

	stagnationCmd := stagnationTickCmd()
	if stagnationCmd == nil {
		t.Fatal("expected stagnation tick command")
	}
	if _, ok := stagnationCmd().(stagnationTickMsg); !ok {
		t.Fatalf("expected stagnationTickMsg, got %T", stagnationCmd())
	}
}

func TestResolveStatusDotStateMappings(t *testing.T) {
	tests := []struct {
		name      string
		item      chatEntry
		wantState statusDotState
		wantShow  bool
	}{
		{
			name:      "tool running",
			item:      chatEntry{Kind: "tool", Status: "running"},
			wantState: dotRunning,
			wantShow:  true,
		},
		{
			name:      "tool success",
			item:      chatEntry{Kind: "tool", Status: "done"},
			wantState: dotSuccess,
			wantShow:  true,
		},
		{
			name:      "tool error",
			item:      chatEntry{Kind: "tool", Status: "failed"},
			wantState: dotError,
			wantShow:  true,
		},
		{
			name:      "tool fallback",
			item:      chatEntry{Kind: "tool", Status: "queued"},
			wantState: dotPending,
			wantShow:  true,
		},
		{
			name:      "assistant text",
			item:      chatEntry{Kind: "assistant", Status: "final"},
			wantState: dotText,
			wantShow:  true,
		},
		{
			name:      "assistant error",
			item:      chatEntry{Kind: "assistant", Status: "error"},
			wantState: dotError,
			wantShow:  true,
		},
		{
			name:      "assistant pending",
			item:      chatEntry{Kind: "assistant", Status: "pending"},
			wantState: dotPending,
			wantShow:  true,
		},
		{
			name:      "assistant hidden",
			item:      chatEntry{Kind: "assistant", Status: "done"},
			wantState: dotHidden,
			wantShow:  false,
		},
		{
			name:      "non chat kind hidden",
			item:      chatEntry{Kind: "user", Status: "final"},
			wantState: dotHidden,
			wantShow:  false,
		},
	}

	m := model{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotShow := m.resolveStatusDotState(tt.item)
			if gotState != tt.wantState || gotShow != tt.wantShow {
				t.Fatalf("expected (%v, %v), got (%v, %v)", tt.wantState, tt.wantShow, gotState, gotShow)
			}
		})
	}
}

func TestRenderStatusDotVisibility(t *testing.T) {
	m := model{dotBlinkVisible: false}
	spacer := stripANSI(dotSpacer())

	hidden := stripANSI(m.renderStatusDot(chatEntry{Kind: "tool", Status: "running"}))
	if hidden != spacer {
		t.Fatalf("expected hidden running dot when blink is off, got %q", hidden)
	}

	m.dotBlinkVisible = true
	runningVisible := stripANSI(m.renderStatusDot(chatEntry{Kind: "tool", Status: "running"}))
	if strings.TrimSpace(runningVisible) == "" {
		t.Fatalf("expected visible running dot when blink is on, got %q", runningVisible)
	}

	m.dotBlinkVisible = false
	assistantVisible := stripANSI(m.renderStatusDot(chatEntry{Kind: "assistant", Status: "final"}))
	if strings.TrimSpace(assistantVisible) == "" {
		t.Fatalf("expected assistant final dot to stay visible, got %q", assistantVisible)
	}
}

func TestColorForDotStateMappings(t *testing.T) {
	m := model{}

	if got := m.colorForDotState(dotSuccess, true); got != semanticColors.Success {
		t.Fatalf("expected success color %q, got %q", semanticColors.Success, got)
	}
	if got := m.colorForDotState(dotError, true); got != semanticColors.Danger {
		t.Fatalf("expected error color %q, got %q", semanticColors.Danger, got)
	}
	if got := m.colorForDotState(dotPending, true); got != semanticColors.TextMuted {
		t.Fatalf("expected pending color %q, got %q", semanticColors.TextMuted, got)
	}
	if got := m.colorForDotState(dotText, true); got != semanticColors.TextBase {
		t.Fatalf("expected text color %q, got %q", semanticColors.TextBase, got)
	}
}

func TestColorForDotStateRunningUsesStagnationColorWhenSolid(t *testing.T) {
	m := model{
		stagnationActive: true,
		stagnationStart:  time.Now().Add(-stagnationThreshold - time.Second),
		reducedMotion:    true,
	}

	if got := m.colorForDotState(dotRunning, true); got != semanticColors.Danger {
		t.Fatalf("expected stagnation running color to reach danger, got %q", got)
	}
	if got := m.colorForDotState(dotRunning, false); got != semanticColors.TextMuted {
		t.Fatalf("expected blinking running color to remain muted, got %q", got)
	}
}

func TestStagnationColorBranches(t *testing.T) {
	if got := (model{}).stagnationColor(); got != semanticColors.TextMuted {
		t.Fatalf("expected zero-start stagnation color to be muted, got %q", got)
	}

	future := model{stagnationStart: time.Now().Add(time.Second)}
	if got := future.stagnationColor(); got != semanticColors.TextMuted {
		t.Fatalf("expected future stagnation start to stay muted, got %q", got)
	}

	reducedBefore := model{
		reducedMotion:   true,
		stagnationStart: time.Now().Add(-stagnationThreshold + 200*time.Millisecond),
	}
	if got := reducedBefore.stagnationColor(); got != semanticColors.TextMuted {
		t.Fatalf("expected reduced-motion color before threshold to stay muted, got %q", got)
	}

	reducedAfter := model{
		reducedMotion:   true,
		stagnationStart: time.Now().Add(-stagnationThreshold - 200*time.Millisecond),
	}
	if got := reducedAfter.stagnationColor(); got != semanticColors.Danger {
		t.Fatalf("expected reduced-motion color after threshold to be danger, got %q", got)
	}

	longElapsed := model{stagnationStart: time.Now().Add(-stagnationTransition - 200*time.Millisecond)}
	if got := longElapsed.stagnationColor(); got != semanticColors.Danger {
		t.Fatalf("expected transition-complete color to be danger, got %q", got)
	}

	inTransition := model{stagnationStart: time.Now().Add(-stagnationTransition / 2)}
	mid := inTransition.stagnationColor()
	if mid == semanticColors.TextMuted || mid == semanticColors.Danger {
		t.Fatalf("expected interpolated color during transition, got %q", mid)
	}
}

func TestHasBlinkingDot(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Status: "final"},
			{Kind: "tool", Status: "done"},
		},
	}
	if m.hasBlinkingDot() {
		t.Fatal("expected no blinking dot without running tools")
	}

	m.chatItems = append(m.chatItems, chatEntry{Kind: "tool", Status: "running"})
	if !m.hasBlinkingDot() {
		t.Fatal("expected blinking dot when a tool is running")
	}
}

func TestHasActiveDotAnimation(t *testing.T) {
	now := time.Now()

	if got := (model{}).hasActiveDotAnimation(); got {
		t.Fatal("expected no active animation without token timestamp")
	}

	reduced := model{
		reducedMotion: true,
		lastTokenTime: now,
		busy:          true,
	}
	if got := reduced.hasActiveDotAnimation(); got {
		t.Fatal("expected reduced-motion mode to disable animation")
	}

	busy := model{
		lastTokenTime: now,
		busy:          true,
	}
	if got := busy.hasActiveDotAnimation(); !got {
		t.Fatal("expected busy model to keep animation active")
	}

	transitioning := model{
		lastTokenTime:    now,
		stagnationActive: true,
		stagnationStart:  now.Add(-stagnationTransition / 2),
	}
	if got := transitioning.hasActiveDotAnimation(); !got {
		t.Fatal("expected active stagnation transition to keep animation active")
	}

	done := model{
		lastTokenTime:    now,
		stagnationActive: true,
		stagnationStart:  now.Add(-stagnationTransition - time.Second),
	}
	if got := done.hasActiveDotAnimation(); got {
		t.Fatal("expected completed stagnation transition to stop animation")
	}
}

func TestIsReducedMotionFromEnvironment(t *testing.T) {
	t.Setenv("BYTEMIND_REDUCED_MOTION", "true")
	t.Setenv("prefers-reduced-motion", "")
	if !isReducedMotion() {
		t.Fatal("expected reduced motion from BYTEMIND_REDUCED_MOTION")
	}

	t.Setenv("BYTEMIND_REDUCED_MOTION", "")
	t.Setenv("prefers-reduced-motion", "on")
	if !isReducedMotion() {
		t.Fatal("expected reduced motion from prefers-reduced-motion")
	}

	t.Setenv("BYTEMIND_REDUCED_MOTION", "")
	t.Setenv("prefers-reduced-motion", "")
	if isReducedMotion() {
		t.Fatal("expected reduced motion off when env flags are absent")
	}
}

func TestUpdateStagnationTransitions(t *testing.T) {
	now := time.Now()

	reduced := model{reducedMotion: true, lastTokenTime: now.Add(-stagnationThreshold)}
	if changed := reduced.updateStagnation(); changed {
		t.Fatal("expected no stagnation updates in reduced motion mode")
	}

	noToken := model{}
	if changed := noToken.updateStagnation(); changed {
		t.Fatal("expected no stagnation updates without token time")
	}

	activate := model{lastTokenTime: now.Add(-stagnationThreshold - time.Second)}
	if changed := activate.updateStagnation(); !changed {
		t.Fatal("expected stagnation activation change")
	}
	if !activate.stagnationActive || activate.stagnationStart.IsZero() {
		t.Fatalf("expected active stagnation with start time, got %#v", activate)
	}

	activate.stagnationStart = now.Add(-stagnationTransition / 2)
	if changed := activate.updateStagnation(); !changed {
		t.Fatal("expected transition animation refresh while stagnating")
	}

	activate.stagnationStart = now.Add(-stagnationTransition - time.Second)
	if changed := activate.updateStagnation(); changed {
		t.Fatal("expected no refresh after stagnation transition completes")
	}

	activate.chatItems = []chatEntry{{Kind: "tool", Status: "running"}}
	if changed := activate.updateStagnation(); !changed {
		t.Fatal("expected active tool to clear stagnation")
	}
	if activate.stagnationActive || !activate.stagnationStart.IsZero() {
		t.Fatalf("expected stagnation to reset when tool becomes active, got %#v", activate)
	}

	activate.stagnationActive = true
	activate.stagnationStart = now.Add(-time.Second)
	activate.chatItems = nil
	activate.lastTokenTime = now
	if changed := activate.updateStagnation(); !changed {
		t.Fatal("expected fresh token to clear stagnation")
	}
	if activate.stagnationActive || !activate.stagnationStart.IsZero() {
		t.Fatalf("expected stagnation reset after fresh token, got %#v", activate)
	}
}

func TestHasActiveToolRunning(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Status: "final"},
			{Kind: "tool", Status: "done"},
		},
	}
	if m.hasActiveToolRunning() {
		t.Fatal("expected no active tool when statuses are not running")
	}

	m.chatItems = append(m.chatItems, chatEntry{Kind: "tool", Status: " active "})
	if !m.hasActiveToolRunning() {
		t.Fatal("expected active tool for trimmed running-like status")
	}
}

func TestResetStagnation(t *testing.T) {
	m := model{
		stagnationActive: true,
		stagnationStart:  time.Now().Add(-time.Second),
	}

	m.resetStagnation()
	if m.lastTokenTime.IsZero() {
		t.Fatal("expected resetStagnation to set lastTokenTime")
	}
	if m.stagnationActive || !m.stagnationStart.IsZero() {
		t.Fatalf("expected resetStagnation to clear stagnation state, got %#v", m)
	}
}

func TestDotColorHelpers(t *testing.T) {
	if got := lerpColorHex("#000000", "#FFFFFF", -1); got != lipgloss.Color("#000000") {
		t.Fatalf("expected clamp-low lerp color #000000, got %q", got)
	}
	if got := lerpColorHex("#000000", "#FFFFFF", 2); got != lipgloss.Color("#FFFFFF") {
		t.Fatalf("expected clamp-high lerp color #FFFFFF, got %q", got)
	}
	if got := lerpColorHex("#000000", "#FFFFFF", 0.5); got != lipgloss.Color("#808080") {
		t.Fatalf("expected midpoint lerp color #808080, got %q", got)
	}

	if got := easeOutCubic(-1); got != 0 {
		t.Fatalf("expected clamped easeOutCubic(-1) to be 0, got %v", got)
	}
	if got := easeOutCubic(2); got != 1 {
		t.Fatalf("expected clamped easeOutCubic(2) to be 1, got %v", got)
	}

	r, g, b := parseHexColor("#ABCDEF")
	if r != 171 || g != 205 || b != 239 {
		t.Fatalf("expected #ABCDEF to parse as 171/205/239, got %v/%v/%v", r, g, b)
	}
	r, g, b = parseHexColor("bad")
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("expected invalid hex to parse as zeros, got %v/%v/%v", r, g, b)
	}

	if got := hexDigit('0'); got != 0 {
		t.Fatalf("expected hexDigit('0') == 0, got %d", got)
	}
	if got := hexDigit('f'); got != 15 {
		t.Fatalf("expected hexDigit('f') == 15, got %d", got)
	}
	if got := hexDigit('B'); got != 11 {
		t.Fatalf("expected hexDigit('B') == 11, got %d", got)
	}
	if got := hexDigit('x'); got != 0 {
		t.Fatalf("expected hexDigit('x') == 0 for invalid value, got %d", got)
	}

	if got := hexColor(-5, 300, 10.4); got != "#00FF0A" {
		t.Fatalf("expected clamped hexColor result #00FF0A, got %q", got)
	}
	if got := rgbToHex(0, 255, 16); got != "#00FF10" {
		t.Fatalf("expected rgbToHex result #00FF10, got %q", got)
	}

	if got := colorToHex(lipgloss.Color("#123456")); got != "#123456" {
		t.Fatalf("expected colorToHex to preserve valid hex, got %q", got)
	}
	if got := colorToHex(lipgloss.Color("red")); got != "red" {
		t.Fatalf("expected non-hex color to pass through, got %q", got)
	}

	if got := clamp01(-0.5); got != 0 {
		t.Fatalf("expected clamp01(-0.5) to be 0, got %v", got)
	}
	if got := clamp01(1.5); got != 1 {
		t.Fatalf("expected clamp01(1.5) to be 1, got %v", got)
	}
	if got := clamp0255(-3); got != 0 {
		t.Fatalf("expected clamp0255(-3) to be 0, got %v", got)
	}
	if got := clamp0255(500); got != 255 {
		t.Fatalf("expected clamp0255(500) to be 255, got %v", got)
	}
}
