package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
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

func TestRunTraceList(t *testing.T) {
	var stdout bytes.Buffer
	err := RunTrace([]string{"list"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
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

func TestTraceShowNoData(t *testing.T) {
	var stdout bytes.Buffer
	err := traceShow("nonexistent_run", &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "No trace found") {
		t.Errorf("expected 'No trace found', got %s", output[:100])
	}
}

func TestTraceExportNoData(t *testing.T) {
	var stdout bytes.Buffer
	err := traceExport("nonexistent_run", "json", &stdout, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestTraceExportUnsupportedFormat(t *testing.T) {
	var stdout bytes.Buffer
	err := traceExport("any", "xml", &stdout, nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestLoadEventsForRunNoAuditDir(t *testing.T) {
	oldHome := os.Getenv("BYTEMIND_HOME")
	defer os.Setenv("BYTEMIND_HOME", oldHome)
	os.Setenv("BYTEMIND_HOME", t.TempDir())

	events, err := loadEventsForRun("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestReadAuditFileWithEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"event_id":"e1","session_id":"s1","action":"test","result":"ok","trace_id":"tr1"}` + "\n" +
		`{"event_id":"e2","session_id":"s2","action":"test","result":"ok","trace_id":"tr2"}` + "\n"
	os.WriteFile(path, []byte(content), 0o644)

	events, err := readAuditFile(path, "tr1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for trace tr1, got %d", len(events))
	}
	if string(events[0].TraceID) != "tr1" {
		t.Fatalf("expected trace tr1, got %s", events[0].TraceID)
	}
}

func TestReadAuditFileAllEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"event_id":"e1","session_id":"s1","action":"test","result":"ok","trace_id":"tr1"}` + "\n" +
		`{"event_id":"e2","session_id":"s2","action":"test","result":"ok","trace_id":"tr2"}` + "\n"
	os.WriteFile(path, []byte(content), 0o644)

	events, err := readAuditFile(path, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for 'all', got %d", len(events))
	}
}

func TestReadAuditFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	content := `not json` + "\n"
	os.WriteFile(path, []byte(content), 0o644)

	events, err := readAuditFile(path, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for bad json, got %d", len(events))
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("a\nb\nc\n"), 0o644)
	if n := countLines(path); n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestCountLinesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0o644)
	if n := countLines(path); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestCountLinesMissingFile(t *testing.T) {
	if n := countLines("nonexistent.txt"); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestTraceExportMarkdownFormat(t *testing.T) {
	home := t.TempDir()
	auditDir := filepath.Join(home, "audit")
	os.MkdirAll(auditDir, 0o755)
	path := filepath.Join(auditDir, "2026-05-13.jsonl")
	content := `{"event_id":"e1","session_id":"s1","trace_id":"tr1","actor":"agent","action":"tool_call","result":"success","timestamp":"2026-05-13T10:00:00Z","metadata":{"tool_name":"read_file"}}` + "\n"
	os.WriteFile(path, []byte(content), 0o644)

	defer os.Setenv("BYTEMIND_HOME", os.Getenv("BYTEMIND_HOME"))
	os.Setenv("BYTEMIND_HOME", home)
	config.ResolveHomeDir()

	var stdout bytes.Buffer
	err := traceExport("tr1", "markdown", &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Agent Trace") {
		t.Errorf("expected markdown header, got %s", output[:100])
	}
	if !strings.Contains(output, "tool_call") {
		t.Errorf("expected tool_call in output, got %s", output[:100])
	}
}

func TestAwayPolicyLabelBranches(t *testing.T) {
	if s := awayPolicyLabel("fail_fast"); s != "deny and fail" {
		t.Fatalf("expected 'deny and fail', got %q", s)
	}
	if s := awayPolicyLabel(""); s != "deny and continue" {
		t.Fatalf("expected 'deny and continue', got %q", s)
	}
	if s := awayPolicyLabel("auto_deny_continue"); s != "deny and continue" {
		t.Fatalf("expected 'deny and continue', got %q", s)
	}
}

func TestWriteAccessSummaryBranches(t *testing.T) {
	if s := writeAccessSummary("", "never"); s != "auto-approved (approval_policy=never)" {
		t.Fatalf("unexpected: %q", s)
	}
	if s := writeAccessSummary("full_access", "on-request"); s != "auto-approved" {
		t.Fatalf("unexpected: %q", s)
	}
	if s := writeAccessSummary("interactive", "on-request"); s != "require confirmation" {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestSafetyReportNewSections(t *testing.T) {
	var stdout bytes.Buffer
	err := RunSafety([]string{"status"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	checks := []string{
		"ByteMind Safety Report",
		"Blocked commands",
		"Network policy",
		"Config file",
		"Max iterations",
	}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("expected safety report to contain %q", c)
		}
	}
}
