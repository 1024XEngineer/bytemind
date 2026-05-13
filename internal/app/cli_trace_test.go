package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunTraceNoArgsShowsUsage(t *testing.T) {
	var stdout bytes.Buffer
	err := RunTrace([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage, got %s", output[:100])
	}
	if !strings.Contains(output, "trace list") {
		t.Errorf("expected trace list in usage, got %s", output[:100])
	}
	if !strings.Contains(output, "trace show") {
		t.Errorf("expected trace show in usage, got %s", output[:100])
	}
	if !strings.Contains(output, "trace export") {
		t.Errorf("expected trace export in usage, got %s", output[:100])
	}
}

func TestRunTraceListExistingData(t *testing.T) {
	var stdout bytes.Buffer
	err := RunTrace([]string{"list"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	// Should either show "No trace data found" or list sessions
	if !strings.Contains(output, "trace sessions") && !strings.Contains(output, "No trace data") {
		t.Errorf("expected trace session list, got %s", output[:100])
	}
}

func TestRunTraceShowMissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunTrace([]string{"show"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
}

func TestRunTraceExportMissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunTrace([]string{"export"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
}

func TestRunTraceUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunTrace([]string{"unknown"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}
