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
	got := renderSemanticAssistantLine("- 工具调用: 读写文件、搜索、打补丁", 80)
	plain := stripANSI(got)
	if !strings.Contains(plain, "- 工具调用: 读写文件、搜索、打补丁") {
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
