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
