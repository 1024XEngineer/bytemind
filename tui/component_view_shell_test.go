package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderLandingProducesContent(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen: screenLanding,
		width:  120,
		height: 36,
		mode:   modeBuild,
		input:  input,
	}
	m.syncInputStyle()

	rendered := m.renderLanding()
	if strings.TrimSpace(rendered) == "" {
		t.Fatalf("expected landing view to render non-empty content")
	}
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Your AI assistant") {
		t.Fatalf("expected landing subtitle in rendered content, got %q", plain)
	}
	if !strings.Contains(plain, "bytemind@localhost:~") {
		t.Fatalf("expected prompt hero header in rendered content, got %q", plain)
	}
	if !strings.Contains(plain, "launching bytemind") {
		t.Fatalf("expected prompt hero launch line in rendered content, got %q", plain)
	}
	if !strings.Contains(plain, "█") {
		t.Fatalf("expected prompt hero pixel matrix logo in rendered content, got %q", plain)
	}
}

func TestOverlayBottomAlignedPlacesOverlayAtBottom(t *testing.T) {
	base := "row1\r\nrow2\r\nrow3"
	overlay := "tail1\r\ntail2"

	out := overlayBottomAligned(base, overlay, 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines after overlay, got %d", len(lines))
	}
	if got := strings.TrimSpace(lines[1]); got != "tail1" {
		t.Fatalf("expected second line to be tail1, got %q", got)
	}
	if got := strings.TrimSpace(lines[2]); got != "tail2" {
		t.Fatalf("expected third line to be tail2, got %q", got)
	}

	trimmed := overlayBottomAligned("a\nb", "x\ny\nz", 2)
	trimmedLines := strings.Split(trimmed, "\n")
	if len(trimmedLines) != 2 {
		t.Fatalf("expected 2 lines after trimming, got %d", len(trimmedLines))
	}
	if got := strings.TrimSpace(trimmedLines[0]); got != "y" {
		t.Fatalf("expected first trimmed line to be y, got %q", got)
	}
	if got := strings.TrimSpace(trimmedLines[1]); got != "z" {
		t.Fatalf("expected second trimmed line to be z, got %q", got)
	}
}

func TestLandingPromptHelpers(t *testing.T) {
	if got := padLandingANSI("abc", 7); got != "abc    " {
		t.Fatalf("expected landing ANSI padding to preserve text and add spaces, got %q", got)
	}
	if got := padLandingANSI("abcdef", 3); got != "abc" {
		t.Fatalf("expected landing ANSI padding to clamp long text, got %q", got)
	}

	m := model{width: 0}
	if got := m.landingPromptHeroWidth(); got != 74 {
		t.Fatalf("expected default prompt hero width 74, got %d", got)
	}

	rows := landingPixelLogoRows("BY", landingModeStyle, landingModeStyle, 0, 120)
	if len(rows) != 7 {
		t.Fatalf("expected 7 pixel logo rows, got %d", len(rows))
	}
	if !strings.Contains(rows[0], "█") {
		t.Fatalf("expected pixel row to contain block glyph, got %q", rows[0])
	}
	narrowRows := landingPixelLogoRows("BYTEMIND", landingModeStyle, landingModeStyle, 0, 50)
	if len(narrowRows) != 7 {
		t.Fatalf("expected 7 compact pixel rows, got %d", len(narrowRows))
	}
	if got := xansi.StringWidth(narrowRows[0]); got > 50 {
		t.Fatalf("expected compact pixel row width <= 50, got %d", got)
	}
}

func TestRenderLandingCanvasUsesLandingContentTop(t *testing.T) {
	m := model{
		width:  30,
		height: 10,
	}
	content := "A\nB"
	rendered := m.renderLandingCanvas(content)
	lines := strings.Split(strings.ReplaceAll(rendered, "\r\n", "\n"), "\n")
	got := -1
	for i, line := range lines {
		if strings.Contains(xansi.Strip(line), "A") {
			got = i
			break
		}
	}
	if got < 0 {
		t.Fatalf("expected rendered canvas to contain content first line")
	}
	want := m.landingContentTop(2)
	if got != want {
		t.Fatalf("expected first content row at %d, got %d", want, got)
	}
}

func TestRenderLandingCanvasClampsRowWidthOnNarrowTerminal(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen: screenLanding,
		width:  72,
		height: 32,
		mode:   modeBuild,
		input:  input,
	}
	m.syncInputStyle()

	rendered := m.renderLandingCanvas(m.renderLandingContent(false))
	lines := strings.Split(strings.ReplaceAll(rendered, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if got := xansi.StringWidth(line); got > m.width {
			t.Fatalf("expected row width <= %d at row %d, got %d", m.width, i, got)
		}
	}
}
