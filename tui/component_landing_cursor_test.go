package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func TestLandingInputCursorMovementDoesNotShiftContent(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("abcdef")
	input.SetCursor(2)

	m := model{
		screen: screenLanding,
		width:  120,
		height: 32,
		input:  input,
	}
	m.syncInputStyle()

	m.landingGlowStep = 0
	visible := xansi.Strip(m.renderLandingInputEditorView())
	if !strings.Contains(visible, "abcdef") {
		t.Fatalf("expected content to stay stable with visible caret, got %q", visible)
	}
	if strings.Contains(visible, "|") {
		t.Fatalf("expected no inserted bar in content with visible caret, got %q", visible)
	}
}

func TestLandingInputCaretBlinksByGlowStep(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("abcdef")
	input.SetCursor(2)

	m := model{
		screen: screenLanding,
		width:  120,
		height: 32,
		input:  input,
	}
	m.syncInputStyle()

	m.landingGlowStep = 0
	if !m.landingCaretVisible() {
		t.Fatalf("expected caret visible at glow step 0")
	}
	visibleRaw := m.renderLandingInputEditorView()
	m.landingGlowStep = 8
	if m.landingCaretVisible() {
		t.Fatalf("expected caret hidden at glow step 8")
	}
	hiddenRaw := m.renderLandingInputEditorView()

	visible := xansi.Strip(visibleRaw)
	hidden := xansi.Strip(hiddenRaw)
	if strings.Contains(visible, "|") || strings.Contains(hidden, "|") {
		t.Fatalf("expected no inserted bar in content, visible=%q hidden=%q", visible, hidden)
	}
	if visible != hidden {
		t.Fatalf("expected stripped content to remain unchanged while blinking, visible=%q hidden=%q", visible, hidden)
	}
	if !strings.Contains(hidden, "abcdef") {
		t.Fatalf("expected input text to remain unchanged when caret hidden, got %q", hidden)
	}
}

func TestLandingInputCaretBlinkKeepsDisplayWidthForWideRune(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("界abc")
	input.SetCursor(0)

	m := model{
		screen: screenLanding,
		width:  120,
		height: 32,
		input:  input,
	}
	m.syncInputStyle()

	m.landingGlowStep = 0
	visible := xansi.Strip(m.renderLandingInputEditorView())
	m.landingGlowStep = 8
	hidden := xansi.Strip(m.renderLandingInputEditorView())

	if strings.Contains(visible, "|") || strings.Contains(hidden, "|") {
		t.Fatalf("expected no inserted bar in wide-rune content, visible=%q hidden=%q", visible, hidden)
	}

	visibleWidth := runewidth.StringWidth(visible)
	hiddenWidth := runewidth.StringWidth(hidden)
	if visibleWidth != hiddenWidth {
		t.Fatalf("expected stable display width during blink, visible=%d hidden=%d, visible=%q hidden=%q", visibleWidth, hiddenWidth, visible, hidden)
	}
}
