package tui

import (
	"strings"
	"testing"
)

func TestFormatChatBodyModeHandlesPasteMarkersForRenderAndCopy(t *testing.T) {
	item := chatEntry{
		Kind: "user",
		Body: "Please inspect [Paste #1 ~12 lines]",
	}

	rendered := stripANSI(formatChatBodyMode(item, 80, false))
	if !strings.Contains(rendered, "[Paste #1 ~12 lines] [click]") {
		t.Fatalf("expected rendered paste marker with click hint, got %q", rendered)
	}

	copied := stripANSI(formatChatBodyMode(item, 80, true))
	if !strings.Contains(copied, "[Paste #1 ~12 lines]") {
		t.Fatalf("expected copied paste marker text to be preserved, got %q", copied)
	}
	if strings.Contains(copied, "[click]") {
		t.Fatalf("expected copy-mode output to omit click hint, got %q", copied)
	}
}

func TestResolveUserBodyPastesRendersCollapsedPreviewAndFullModes(t *testing.T) {
	m := model{
		pastedContents: map[string]pastedContent{
			"1": {
				ID:      "1",
				Content: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12",
				Lines:   12,
			},
		},
		pasteExpandLevel: map[string]int{},
	}

	collapsed := stripANSI(m.resolveUserBodyPastes("Before [Paste #1 ~12 lines] after"))
	if !strings.Contains(collapsed, "Before [Paste #1 ~12 lines] after") {
		t.Fatalf("expected collapsed paste body to preserve marker inline, got %q", collapsed)
	}

	m.pasteExpandLevel["1"] = 1
	preview := stripANSI(m.resolveUserBodyPastes("[Paste #1 ~12 lines]"))
	for _, want := range []string{"[Paste #1 ~12 lines] [preview]", "line1", "line10", "Ctrl+E expand all"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("expected preview paste body to contain %q, got %q", want, preview)
		}
	}
	if strings.Contains(preview, "line12") {
		t.Fatalf("expected preview paste body to hide final lines, got %q", preview)
	}

	m.pasteExpandLevel["1"] = 2
	full := stripANSI(m.resolveUserBodyPastes("[Paste #1 ~12 lines]"))
	for _, want := range []string{"[Paste #1 ~12 lines] [full]", "line12", "click again to collapse"} {
		if !strings.Contains(full, want) {
			t.Fatalf("expected full paste body to contain %q, got %q", want, full)
		}
	}
}

func TestRenderUserPasteAwareLineHandlesPreviewSuffixAndWrappedHint(t *testing.T) {
	line := "Before [Paste #7 ~3 lines] [preview] after"
	got := stripANSI(strings.Join(renderUserPasteAwareLine(line, 18, false), "\n"))
	for _, want := range []string{"Before", "[Paste #7 ~3", "lines] [preview]", "after"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected preview-suffixed paste line to contain %q, got %q", want, got)
		}
	}

	wrapped := stripANSI(strings.Join(renderWrappedPasteMarker("[Paste #9 ~20 lines]", 10, false, true), "\n"))
	if !strings.Contains(wrapped, "[click]") {
		t.Fatalf("expected wrapped paste marker to retain click hint, got %q", wrapped)
	}
}

func TestResolvePasteHelpersCoverShortAndFallbackBranches(t *testing.T) {
	content := pastedContent{
		ID:      "3",
		Content: "alpha\nbeta",
		Lines:   2,
	}

	if got := stripANSI(resolvePasteBlockPlain(content, 0)); !strings.Contains(got, "[Paste #3 ~2 lines]") {
		t.Fatalf("expected collapsed plain paste block marker, got %q", got)
	}
	if got := stripANSI(resolvePasteBlockPlain(content, 2)); got != "alpha\nbeta" {
		t.Fatalf("expected full plain paste block content, got %q", got)
	}

	preview := stripANSI(resolvePastePreviewPlain(content))
	if strings.Contains(preview, "Ctrl+E") {
		t.Fatalf("expected short plain preview not to render expansion hint, got %q", preview)
	}

	framedPreview := stripANSI(renderResolvedPastePreviewPlain(content, "[Paste #3 ~2 lines]"))
	if !strings.Contains(framedPreview, "click again to show full content") {
		t.Fatalf("expected short framed preview to render short-content hint, got %q", framedPreview)
	}

	framedCollapsed := stripANSI(renderResolvedPasteBlockPlain(content, 0))
	if framedCollapsed != "[Paste #3 ~2 lines]" {
		t.Fatalf("expected collapsed framed paste block to return header only, got %q", framedCollapsed)
	}
}

func TestRenderWrappedPasteMarkerBranchCoverage(t *testing.T) {
	if got := renderWrappedPasteMarker("", 20, false, true); got != nil {
		t.Fatalf("expected empty marker to produce nil output, got %#v", got)
	}

	copyMode := stripANSI(strings.Join(renderWrappedPasteMarker("[Paste #2 ~9 lines]", 12, true, true), "\n"))
	if !strings.Contains(copyMode, "[Paste #2") || !strings.Contains(copyMode, "~9 lines]") || strings.Contains(copyMode, "[click]") {
		t.Fatalf("expected copy mode to keep marker text without click hint, got %q", copyMode)
	}

	noHint := stripANSI(strings.Join(renderWrappedPasteMarker("[Paste #2 ~9 lines]", 12, false, false), "\n"))
	if strings.Contains(noHint, "[click]") {
		t.Fatalf("expected no-hint mode to omit click hint, got %q", noHint)
	}
}

func TestResolveUserBodyPastesFallbackBranches(t *testing.T) {
	m := model{
		pastedContents: map[string]pastedContent{
			"1": {ID: "1", Content: "ok", Lines: 1},
		},
		pasteExpandLevel: map[string]int{"1": 2},
	}

	noID := stripANSI(m.resolveUserBodyPastes("[Paste ~12 lines]"))
	if noID != "[Paste ~12 lines]" {
		t.Fatalf("expected marker without id to remain unchanged, got %q", noID)
	}

	missing := stripANSI(m.resolveUserBodyPastes("x [Paste #99 ~3 lines] y"))
	if missing != "x [Paste #99 ~3 lines] y" {
		t.Fatalf("expected missing stored paste to keep marker text, got %q", missing)
	}
}
