package tui

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderLandingProducesContent(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:    screenLanding,
		width:     120,
		height:    36,
		mode:      modeBuild,
		input:     input,
		workspace: `D:\happycoding\lzy1\byte-lab`,
		version:   "v1.2.3",
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "gpt-5.4-mini"},
		},
	}
	m.syncInputStyle()

	rendered := m.renderLanding()
	if strings.TrimSpace(rendered) == "" {
		t.Fatalf("expected landing view to render non-empty content")
	}
	plain := xansi.Strip(rendered)
	if got := strings.Count(plain, "Your AI assistant"); got != 1 {
		t.Fatalf("expected landing assistant label once in rendered content, got %d in %q", got, plain)
	}
	if !strings.Contains(plain, "./byte-lab") {
		t.Fatalf("expected workspace name in prompt hero header, got %q", plain)
	}
	if !strings.Contains(plain, "gpt-5.4-mini") {
		t.Fatalf("expected current model in landing mode row, got %q", plain)
	}
	if !strings.Contains(plain, "v1.2.3") {
		t.Fatalf("expected version in landing canvas, got %q", plain)
	}
	if !strings.Contains(plain, "[ Build ]") {
		t.Fatalf("expected active build tab in landing mode row, got %q", plain)
	}
	if !strings.Contains(plain, landingInputPlaceholder) {
		t.Fatalf("expected action-oriented landing placeholder, got %q", plain)
	}
	for _, want := range []string{"[Enter] send", "[Ctrl+J] newline", "[/] commands", "[Ctrl+L] sessions", "[Ctrl+C] quit"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected landing shortcut hint %q, got %q", want, plain)
		}
	}
	if strings.Contains(plain, "bytemind@localhost") {
		t.Fatalf("expected prompt hero header not to use localhost label, got %q", plain)
	}
	if strings.Contains(plain, "launching bytemind") {
		t.Fatalf("expected prompt hero launch line to use assistant label, got %q", plain)
	}
	if strings.Contains(plain, "Let's get started") {
		t.Fatalf("expected old landing placeholder to be removed, got %q", plain)
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
	wide := model{width: 120}
	if got, want := wide.landingPromptHeroWidth(), wide.landingInputShellWidth(); got != want {
		t.Fatalf("expected landing hero width to align with input width %d, got %d", want, got)
	}
	restoredRows := landingPixelLogoRows(landingLogoText, landingModeStyle, landingModeStyle, 0, wide.landingPromptHeroWidth()-2)
	if got, want := xansi.StringWidth(restoredRows[0]), landingPreferredLogoWidth(landingLogoText); got != want {
		t.Fatalf("expected restored-width logo to keep stable pixel width %d, got %d", want, got)
	}
	for _, tc := range []struct {
		workspace string
		want      string
	}{
		{workspace: `D:\happycoding\lzy1\bytemind`, want: "./bytemind"},
		{workspace: "/home/me/byte-lab/", want: "./byte-lab"},
		{workspace: "", want: "./workspace"},
	} {
		if got := landingWorkspaceName(tc.workspace); got != tc.want {
			t.Fatalf("expected workspace name %q for %q, got %q", tc.want, tc.workspace, got)
		}
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

func TestRenderLandingHeroPlacesVersionInHeader(t *testing.T) {
	m := model{
		width:   30,
		height:  5,
		version: "v1.2.3",
	}
	hero := xansi.Strip(m.renderLandingHero())
	if !strings.Contains(hero, "v1.2.3") {
		t.Fatalf("expected landing hero header to contain version, got %q", hero)
	}
	if strings.Contains(hero, "●") {
		t.Fatalf("expected landing hero header to replace mac-style dots with version, got %q", hero)
	}
}

func TestLandingModeAccentColors(t *testing.T) {
	build := model{screen: screenLanding, mode: modeBuild}
	if got, want := build.modeAccentColor(), lipgloss.Color(landingBuildAccent); got != want {
		t.Fatalf("expected landing build input accent %q, got %q", want, got)
	}
	if got, want := landingModeBuildActiveStyle.GetForeground(), lipgloss.Color(landingBuildAccent); got != want {
		t.Fatalf("expected landing build tab accent %q, got %q", want, got)
	}

	plan := model{screen: screenLanding, mode: modePlan}
	if got, want := plan.modeAccentColor(), lipgloss.Color(landingPlanAccent); got != want {
		t.Fatalf("expected landing plan input accent %q, got %q", want, got)
	}
	if got, want := landingModePlanActiveStyle.GetForeground(), lipgloss.Color(landingPlanAccent); got != want {
		t.Fatalf("expected landing plan tab accent %q, got %q", want, got)
	}
	if got, want := landingModePlanActiveStyle.GetBackground(), lipgloss.Color(landingPlanBg); got != want {
		t.Fatalf("expected landing plan tab background %q, got %q", want, got)
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
