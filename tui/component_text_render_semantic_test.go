package tui

import (
	"strings"
	"testing"
)

func TestSemanticIntentRecognizesKeyLabels(t *testing.T) {
	cases := map[string]string{
		"Warning: careful": "warning",
		"Caution: careful": "warning",
		"Error: boom":      "error",
		"Failure: boom":    "error",
		"Success: done":    "success",
		"Done: finished":   "success",
		"Tip: try this":    "info",
		"Note: remember":   "info",
		"Info: heads up":   "info",
	}

	for input, want := range cases {
		if got := semanticIntent(input); got != want {
			t.Fatalf("semanticIntent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSemanticIntentDoesNotMisclassifyPlainText(t *testing.T) {
	cases := []string{
		"This is a normal sentence.",
		"Noteworthy details follow below.",
		"Successful retries depend on timing.",
	}

	for _, input := range cases {
		if got := semanticIntent(input); got != "" {
			t.Fatalf("semanticIntent(%q) = %q, want empty", input, got)
		}
	}
}

func TestRenderMarkdownHeadingAddsVisualPrefixes(t *testing.T) {
	got := renderMarkdownHeading("## Section", 40)
	if !strings.Contains(got, "\u25c6 Section") {
		t.Fatalf("expected heading prefix in rendered heading, got %q", got)
	}
}

func TestApplyLineIntentStyleColorsInfoWarningAndError(t *testing.T) {
	info := applyLineIntentStyle("Tip: remember this", "Tip: remember this")
	if !strings.Contains(info, "Tip: remember this") {
		t.Fatalf("expected info styling to preserve text, got %q", info)
	}

	warning := applyLineIntentStyle("Warning: careful", "Warning: careful")
	if !strings.Contains(warning, "Warning: careful") {
		t.Fatalf("expected warning styling to preserve text, got %q", warning)
	}

	errText := applyLineIntentStyle("Error: broken", "Error: broken")
	if !strings.Contains(errText, "Error: broken") {
		t.Fatalf("expected error styling to preserve text, got %q", errText)
	}
}

func TestRenderSemanticAssistantLineDoesNotAccentGenericColonLabels(t *testing.T) {
	got := renderSemanticAssistantLine("- Tool call: read files, search, patch", 80)
	plain := stripANSI(got)
	if !strings.Contains(plain, "- Tool call: read files, search, patch") {
		t.Fatalf("expected generic label text to be preserved, got %q", plain)
	}
	if got != plain {
		t.Fatalf("expected generic colon label not to receive standalone semantic highlighting, got %q", got)
	}
}

func TestRenderSemanticAssistantLineKeepsIntentLabelsStyled(t *testing.T) {
	got := stripANSI(renderSemanticAssistantLine("Tip: remember this detail", 12))
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected intent label rendering to wrap with semantic indentation, got %q", got)
	}
	if !strings.HasPrefix(lines[0], "Tip: ") {
		t.Fatalf("expected first line to keep intent label prefix, got %q", got)
	}
	if !strings.HasPrefix(lines[1], strings.Repeat(" ", len("Tip: "))) {
		t.Fatalf("expected wrapped intent label body to align after label prefix, got %q", got)
	}
}

func TestRenderLegacyFencedCodeBlockHandlesEmptyBlock(t *testing.T) {
	got := renderLegacyFencedCodeBlock(nil, 24)
	plain := stripANSI(got)
	topLeft := string(rune(0x256d))
	bottomLeft := string(rune(0x2570))
	if !strings.Contains(plain, topLeft) || !strings.Contains(plain, bottomLeft) {
		t.Fatalf("expected framed empty code block, got %q", plain)
	}
}

func TestRenderLegacyFencedCodeBlockPreservesBlankLinesAndWrapsLongLines(t *testing.T) {
	got := renderLegacyFencedCodeBlock([]string{
		"short",
		"",
		"this is a very long code line that should wrap in the legacy fenced renderer",
	}, 16)
	plain := stripANSI(got)

	if !strings.Contains(plain, "short") {
		t.Fatalf("expected first code line, got %q", plain)
	}
	if !strings.Contains(plain, "very long") || !strings.Contains(plain, "legacy") || !strings.Contains(plain, "fenced") {
		t.Fatalf("expected wrapped long line fragments, got %q", plain)
	}
}

func TestRenderAssistantBodyLegacyRendersFencedCodeAsSingleFrame(t *testing.T) {
	input := strings.Join([]string{
		"before",
		"```go",
		"line one",
		"",
		"line two is very very long and should wrap",
		"```",
		"after",
	}, "\n")
	got := stripANSI(renderAssistantBodyLegacy(input, 20))

	topLeft := string(rune(0x256d))
	bottomLeft := string(rune(0x2570))
	if strings.Count(got, topLeft) != 1 || strings.Count(got, bottomLeft) != 1 {
		t.Fatalf("expected a single code frame, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected non-code content to be preserved, got %q", got)
	}
}

func TestRenderAssistantBodyLegacyUnclosedFenceFlushesAtEOF(t *testing.T) {
	input := strings.Join([]string{
		"```",
		"alpha",
		"beta",
	}, "\n")
	got := stripANSI(renderAssistantBodyLegacy(input, 24))

	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Fatalf("expected unclosed fenced code content rendered, got %q", got)
	}
	topLeft := string(rune(0x256d))
	bottomLeft := string(rune(0x2570))
	if strings.Count(got, topLeft) != 1 || strings.Count(got, bottomLeft) != 1 {
		t.Fatalf("expected single framed block for unclosed fence, got %q", got)
	}
}
