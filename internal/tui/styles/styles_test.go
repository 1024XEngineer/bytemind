package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStyleContracts(t *testing.T) {
	if ColorAccent == lipgloss.Color("") {
		t.Fatalf("expected accent color to be configured")
	}
	if PanelStyle.GetHorizontalFrameSize() <= 0 {
		t.Fatalf("expected panel style to include border/padding frame")
	}
	if ModalTitleStyle.Render("Title") == "" {
		t.Fatalf("expected modal title style to render text")
	}
	if line := CommandPaletteMetaStyle.Render("meta"); !strings.Contains(line, "meta") {
		t.Fatalf("expected command palette meta style to preserve text")
	}
}

func TestSpacer(t *testing.T) {
	if Spacer(0) != "" {
		t.Fatalf("expected zero-width spacer to be empty")
	}
	if got := Spacer(4); lipgloss.Width(got) != 4 {
		t.Fatalf("expected spacer width 4, got %d", lipgloss.Width(got))
	}
}
