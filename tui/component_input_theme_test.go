package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderInputEditorViewDefaultUsesLandingVariantOnLandingScreen(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("hello")
	input.SetCursor(2)

	m := model{
		screen:          screenLanding,
		landingGlowStep: 0,
		input:           input,
	}
	m.syncInputStyle()

	got := xansi.Strip(renderInputEditorViewDefault(m))
	want := xansi.Strip(m.renderLandingInputEditorView())
	if got != want {
		t.Fatalf("expected landing renderer output, got %q want %q", got, want)
	}
}

func TestApplyInputThemesAllowNilModelReceiver(t *testing.T) {
	var m *model
	m.applyInputThemeForScreen()
	m.applyDefaultInputTheme()
	m.applyLandingInputTheme()
}

func TestLandingInputShellWidthCapsToAvailableWidth(t *testing.T) {
	m := model{width: 40}
	if got := m.landingInputShellWidth(); got != 24 {
		t.Fatalf("expected shell width 24 for narrow terminal width 40, got %d", got)
	}
}

func TestLandingInputContentWidthNeverExceedsShell(t *testing.T) {
	m := model{width: 40}
	shell := m.landingInputShellWidth()
	content := m.landingInputContentWidth()
	frame := landingInputStyle.GetHorizontalFrameSize()
	if content > max(1, shell-frame) {
		t.Fatalf("expected content width <= shell-frame, shell=%d frame=%d content=%d", shell, frame, content)
	}
}

func TestApplyDefaultInputThemeResetsLandingCursorStyle(t *testing.T) {
	input := textarea.New()
	m := model{
		screen: screenLanding,
		input:  input,
	}

	m.syncInputStyle()
	if !m.input.Cursor.Style.GetBold() {
		t.Fatalf("expected landing theme cursor to be bold")
	}
	if m.input.Cursor.Style.GetBackground() == nil || m.input.Cursor.Style.GetForeground() == nil {
		t.Fatalf("expected landing theme cursor to apply explicit colors")
	}

	m.screen = screenChat
	m.syncInputStyle()
	defaultCursorStyle := textarea.New().Cursor.Style
	if m.input.Cursor.Style.GetBold() != defaultCursorStyle.GetBold() {
		t.Fatalf("expected default theme to restore cursor bold setting")
	}
	if (m.input.Cursor.Style.GetBackground() == nil) != (defaultCursorStyle.GetBackground() == nil) {
		t.Fatalf("expected default theme to restore cursor background setting")
	}
	if (m.input.Cursor.Style.GetForeground() == nil) != (defaultCursorStyle.GetForeground() == nil) {
		t.Fatalf("expected default theme to restore cursor foreground setting")
	}
}
