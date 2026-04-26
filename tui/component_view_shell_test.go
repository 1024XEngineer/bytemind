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

func TestLandingHelperColorAndCenterFunctions(t *testing.T) {
	if got := centeredText("abc", 7); got != "  abc  " {
		t.Fatalf("expected centered text with symmetric padding, got %q", got)
	}
	if got := centeredText("   abc   ", 3); got != "abc" {
		t.Fatalf("expected trimmed text when width is tight, got %q", got)
	}

	r, g, b := parseHexColor("#0A10FF")
	if r != 10 || g != 16 || b != 255 {
		t.Fatalf("expected parsed RGB 10/16/255, got %d/%d/%d", r, g, b)
	}
	r, g, b = parseHexColor("#123")
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("expected invalid hex to parse as 0/0/0, got %d/%d/%d", r, g, b)
	}

	if got := formatHexColor(300, -5, 16); got != "#FF0010" {
		t.Fatalf("expected clamped hex color #FF0010, got %q", got)
	}
	if got := string(lerpHexColor("#000000", "#FFFFFF", 0.5)); got != "#7F7F7F" {
		t.Fatalf("expected midpoint hex color #7F7F7F, got %q", got)
	}
	if got := string(lerpHexColor("#000000", "#FFFFFF", 2)); got != "#FFFFFF" {
		t.Fatalf("expected t>1 to clamp to end color, got %q", got)
	}
}

func TestLandingFrameGlowPositionPhases(t *testing.T) {
	inactive := landingFrameGlowPosition(0, 5, 3, false)
	if inactive.topCol != -1 || inactive.leftRow != -1 || inactive.rightRow != -1 {
		t.Fatalf("expected inactive glow state to be disabled, got %+v", inactive)
	}

	down := landingFrameGlowPosition(1, 5, 3, true)
	if down.rightRow != 1 {
		t.Fatalf("expected right-side downward phase at row 1, got %+v", down)
	}

	up := landingFrameGlowPosition(3, 5, 3, true)
	if up.rightRow != 1 {
		t.Fatalf("expected right-side upward phase at row 1, got %+v", up)
	}

	top := landingFrameGlowPosition(5, 5, 3, true)
	if top.topCol != 8 {
		t.Fatalf("expected top phase to start from rightmost column 8, got %+v", top)
	}

	left := landingFrameGlowPosition(14, 5, 3, true)
	if left.leftRow != 1 {
		t.Fatalf("expected left-side downward phase at row 1, got %+v", left)
	}
}

func TestLandingFrameTravelFramesAndFrameLineRendering(t *testing.T) {
	if got := landingFrameTravelFrames(0, 3); got != 1 {
		t.Fatalf("expected degenerate travel frames to clamp to 1, got %d", got)
	}
	if got := landingFrameTravelFrames(5, 3); got != 16 {
		t.Fatalf("expected travel frames 16 for width=5 rows=3, got %d", got)
	}

	line := "|--|"
	plain := renderLandingFrameLine(line, -1)
	if stripped := xansi.Strip(plain); stripped != line {
		t.Fatalf("expected plain frame line text to be preserved, got %q", stripped)
	}
	glow := renderLandingFrameLine(line, 1)
	if stripped := xansi.Strip(glow); stripped != line {
		t.Fatalf("expected glow frame line text to be preserved, got %q", stripped)
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
