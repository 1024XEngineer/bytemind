package tui

import (
	"strings"
	"testing"
)

func TestSummarizeToolForWebSearchAndWebFetch(t *testing.T) {
	summary, lines, status := summarizeTool("web_search", `{"query":"go release notes","results":[{"title":"Go 1.23 Release Notes","url":"https://go.dev/doc/devel/release"},{"title":"","url":"https://example.com/fallback"}]}`)
	if status != "done" {
		t.Fatalf("expected done status, got %q", status)
	}
	if !strings.Contains(summary, `Searched web for "go release notes"`) {
		t.Fatalf("unexpected search summary %q", summary)
	}
	if len(lines) != 2 {
		t.Fatalf("expected two preview lines, got %+v", lines)
	}
	if !strings.Contains(lines[0], "Go 1.23 Release Notes") || !strings.Contains(lines[1], "https://example.com/fallback") {
		t.Fatalf("unexpected search lines %+v", lines)
	}

	summary, lines, status = summarizeTool("web_fetch", `{"url":"https://go.dev/doc/devel/release","status_code":200,"title":"Release Notes","content":"A long body preview","truncated":true}`)
	if status != "done" {
		t.Fatalf("expected done status, got %q", status)
	}
	if !strings.Contains(summary, "Fetched https://go.dev/doc/devel/release (HTTP 200)") {
		t.Fatalf("unexpected fetch summary %q", summary)
	}
	if len(lines) < 3 {
		t.Fatalf("expected title/preview/truncated lines, got %+v", lines)
	}
	if !strings.Contains(lines[0], "title: Release Notes") || !strings.Contains(lines[1], "preview: A long body preview") || lines[2] != "content truncated" {
		t.Fatalf("unexpected fetch lines %+v", lines)
	}
}

