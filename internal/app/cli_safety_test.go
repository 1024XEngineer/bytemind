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
