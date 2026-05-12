package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunSafetyStatusOutput(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	expected := []string{
		"ByteMind Safety Status",
		"Approval policy",
		"Approval mode",
		"Writable roots",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("expected safety status to contain %q", s)
		}
	}
}

func TestRunSafetyExplainOutput(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	expected := []string{
		"ByteMind Safety Model",
		"Tool Safety Classes",
		"Approval Policy",
		"Sandbox",
		"Writable Roots",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("expected safety explain to contain %q", s)
		}
	}
}

func TestRunSafetyDefaultShowsStatus(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Status") {
		t.Errorf("expected default safety to show status, got %s", output[:60])
	}
}

func TestRunSafetyStatusWithFlagsAfterSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	// This was broken: "status -workspace <path>" would ignore -workspace
	err := RunSafety([]string{"status", "-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Status") {
		t.Errorf("expected safety status output, got %s", output[:60])
	}
}

func TestRunSafetyExplainWithFlagsAfterSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain", "-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ByteMind Safety Model") {
		t.Errorf("expected safety explain output, got %s", output[:60])
	}
}

func TestRunSafetyStatusContainsShelAllowlist(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Shell allowlist") {
		t.Errorf("expected Shell allowlist section, got %s", output[:100])
	}
}

func TestRunSafetyStatusContainsAccessSummary(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Access summary") {
		t.Errorf("expected Access summary section, got %s", output[:100])
	}
}

func TestRunSafetyExplainContainsWhatIsNotProtected(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "NOT protected") {
		t.Errorf("expected 'NOT protected' section, got %s", output[:200])
	}
}

func TestRunSafetyExplainContainsEmoji(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"explain"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "\u2705") {
		t.Errorf("expected checkmark emoji in safety explain output")
	}
}
