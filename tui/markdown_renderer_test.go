package tui

import (
	"strings"
	"testing"
)

func TestRenderStructuredMarkdownPreparedFormatsLinksAndInlineCode(t *testing.T) {
	result, err := renderStructuredMarkdownPrepared(
		markdownSurfaceAssistant,
		"Read [docs](https://example.com/docs) and run `go test ./...`.",
		72,
	)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	if !strings.Contains(result.Display, "docs") || !strings.Contains(result.Display, "https://example.com/docs") {
		t.Fatalf("expected rendered link text and url, got %q", result.Display)
	}
	if !strings.Contains(result.Display, "go test ./...") {
		t.Fatalf("expected inline code content in display, got %q", result.Display)
	}
	if strings.Contains(result.Copy, "[docs]") || strings.Contains(result.Copy, "`") {
		t.Fatalf("expected markdown tokens to be removed from copy, got %q", result.Copy)
	}
	if !strings.Contains(result.Copy, "docs (https://example.com/docs)") {
		t.Fatalf("expected normalized link in copy, got %q", result.Copy)
	}
}

func TestRenderStructuredMarkdownPreparedShowsLanguageBadgeForUnclosedCodeFence(t *testing.T) {
	result, err := renderStructuredMarkdownPrepared(
		markdownSurfaceAssistant,
		prepareMarkdownInput(markdownSurfaceAssistant, "```go\nfmt.Println(\"hi\")"),
		64,
	)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	if !strings.Contains(result.Display, "[go]") {
		t.Fatalf("expected code language badge in display, got %q", result.Display)
	}
	if !strings.Contains(result.Display, "╭") || !strings.Contains(result.Display, "╰") {
		t.Fatalf("expected code block frame in display, got %q", result.Display)
	}
	if !strings.Contains(result.Display, "fmt.Println(\"hi\")") {
		t.Fatalf("expected code contents in display, got %q", result.Display)
	}
	if !strings.Contains(result.Copy, "[go]") || !strings.Contains(result.Copy, "fmt.Println(\"hi\")") {
		t.Fatalf("expected copy to keep code metadata and content, got %q", result.Copy)
	}
}

func TestRenderStructuredMarkdownPreparedRendersTableGridAndStackedFallback(t *testing.T) {
	input := strings.Join([]string{
		"| Name | Status | Notes |",
		"| --- | --- | --- |",
		"| ByteMind | Ready | compact terminal renderer |",
	}, "\n")

	wide, err := renderStructuredMarkdownPrepared(markdownSurfaceAssistant, input, 72)
	if err != nil {
		t.Fatalf("wide renderStructuredMarkdownPrepared: %v", err)
	}
	if !strings.Contains(wide.Display, "┌") || !strings.Contains(wide.Display, "│") || !strings.Contains(wide.Display, "└") {
		t.Fatalf("expected framed grid table in wide display, got %q", wide.Display)
	}
	if !strings.Contains(wide.Copy, " | ") {
		t.Fatalf("expected plain table separator in wide copy, got %q", wide.Copy)
	}

	narrow, err := renderStructuredMarkdownPrepared(markdownSurfaceAssistant, input, 18)
	if err != nil {
		t.Fatalf("narrow renderStructuredMarkdownPrepared: %v", err)
	}
	for _, want := range []string{"Name: ByteMind", "Status: Ready", "Notes:"} {
		if !strings.Contains(narrow.Copy, want) {
			t.Fatalf("expected stacked fallback copy to contain %q, got %q", want, narrow.Copy)
		}
	}
}

func TestRenderStructuredMarkdownPreparedKeepsDisplayAndCopyLineCountsAligned(t *testing.T) {
	input := strings.Join([]string{
		"## Summary",
		"",
		"- first item with [link](https://example.com)",
		"- second item",
		"",
		"> quoted line",
		"",
		"```go",
		"func main() {",
		"    fmt.Println(\"hi\")",
		"}",
		"```",
	}, "\n")

	result, err := renderStructuredMarkdownPrepared(markdownSurfaceAssistant, prepareMarkdownInput(markdownSurfaceAssistant, input), 42)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	displayLines := strings.Split(strings.ReplaceAll(result.Display, "\r\n", "\n"), "\n")
	copyLines := strings.Split(strings.ReplaceAll(result.Copy, "\r\n", "\n"), "\n")
	if len(displayLines) != len(copyLines) {
		t.Fatalf("expected aligned display/copy line counts, got display=%d copy=%d\nDISPLAY:\n%q\nCOPY:\n%q", len(displayLines), len(copyLines), result.Display, result.Copy)
	}
}

func TestFormatChatCopyBodyUsesPlainMarkdownCopy(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "Use [docs](https://example.com/docs).\n\n```go\nfmt.Println(\"hi\")\n```",
	}

	got := formatChatCopyBody(item, 72)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected copy body to omit ANSI escapes, got %q", got)
	}
	if !strings.Contains(got, "docs (https://example.com/docs)") {
		t.Fatalf("expected normalized link in copy body, got %q", got)
	}
}

func TestRenderStructuredMarkdownPreparedStylesProcessedLineWithoutPlaceholderLeak(t *testing.T) {
	result, err := renderStructuredMarkdownPrepared(
		markdownSurfaceAssistant,
		"Hello there.\n\nProcessed for 9s",
		48,
	)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	if strings.Contains(result.Display, "ZZBYTEMINDTAG") {
		t.Fatalf("expected no semantic placeholder leak, got %q", result.Display)
	}
	if !strings.Contains(stripANSI(result.Display), "Processed for 9s") {
		t.Fatalf("expected rendered processed line text, got %q", result.Display)
	}
	if !strings.Contains(result.Copy, "Processed for 9s") {
		t.Fatalf("expected copy to preserve processed line text, got %q", result.Copy)
	}
}

func TestRenderStructuredMarkdownPreparedRendersNestedListsAndHeadings(t *testing.T) {
	input := strings.Join([]string{
		"# Plan",
		"",
		"1. Ship renderer tokens",
		"   - unify colors",
		"   - add badges",
		"2. Verify output",
	}, "\n")

	result, err := renderStructuredMarkdownPrepared(markdownSurfaceAssistant, input, 72)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	for _, want := range []string{"Plan", "1. Ship renderer tokens", "unify colors", "add badges", "2. Verify output"} {
		if !strings.Contains(result.Copy, want) {
			t.Fatalf("expected nested markdown copy to contain %q, got %q", want, result.Copy)
		}
	}
	for _, want := range []string{"Plan", "1. Ship renderer tokens", "unify colors", "add badges"} {
		if !strings.Contains(result.Display, want) {
			t.Fatalf("expected styled heading/list display to contain %q, got %q", want, result.Display)
		}
	}
}

func TestRenderStructuredMarkdownPreparedStylesToolSurfaceAndCopy(t *testing.T) {
	input := strings.Join([]string{
		"## Summary",
		"",
		"- renderer tokenized",
		"- copy stays plain",
	}, "\n")

	result, err := renderStructuredMarkdownPrepared(markdownSurfaceTool, input, 72)
	if err != nil {
		t.Fatalf("renderStructuredMarkdownPrepared: %v", err)
	}

	for _, want := range []string{"Summary", "renderer tokenized", "copy stays plain"} {
		if !strings.Contains(result.Display, want) {
			t.Fatalf("expected tool surface display to contain %q, got %q", want, result.Display)
		}
	}
	for _, want := range []string{"Summary", "renderer tokenized", "copy stays plain"} {
		if !strings.Contains(result.Copy, want) {
			t.Fatalf("expected tool copy to contain %q, got %q", want, result.Copy)
		}
	}
}

func TestFormatChatCopyBodyUsesPlainToolMarkdownCopy(t *testing.T) {
	item := chatEntry{
		Kind: "tool",
		Body: "## Summary\n\n- scanned renderer\n- kept copy plain",
	}

	got := formatChatCopyBody(item, 72)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected tool copy body to omit ANSI escapes, got %q", got)
	}
	for _, want := range []string{"Summary", "scanned renderer", "kept copy plain"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected tool copy body to contain %q, got %q", want, got)
		}
	}
}
