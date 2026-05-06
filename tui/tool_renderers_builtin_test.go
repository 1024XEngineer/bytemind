package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type emptyResultRenderer struct{}

func (emptyResultRenderer) DisplayLabel() string { return "TEST" }

func (emptyResultRenderer) Render(string) ToolRenderResult { return ToolRenderResult{} }

func TestBuiltinToolRenderersRegistered(t *testing.T) {
	tests := map[string]string{
		"read_file":       "READ",
		"list_files":      "LIST",
		"search_text":     "SEARCH",
		"run_shell":       "SHELL",
		"write_file":      "WRITE",
		"replace_in_file": "EDIT",
		"apply_patch":     "PATCH",
		"update_plan":     "PLAN",
		"web_search":      "SEARCH",
		"web_fetch":       "FETCH",
	}

	for name, wantLabel := range tests {
		renderer := GetToolRenderer(name)
		if renderer == nil {
			t.Fatalf("expected renderer %q to be registered", name)
		}
		if got := renderer.DisplayLabel(); got != wantLabel {
			t.Fatalf("renderer %q label mismatch: got %q want %q", name, got, wantLabel)
		}
	}
}

func TestRenderToolPayloadUnknownToolUsesDefaultRenderer(t *testing.T) {
	payload := "  fallback output line  "

	got := renderToolPayload("unknown_tool_for_test", payload)
	if got.Status != "done" {
		t.Fatalf("expected default status done, got %q", got.Status)
	}
	if got.Summary != "fallback output line" {
		t.Fatalf("expected trimmed fallback summary, got %q", got.Summary)
	}
	if got.CompactLine != "fallback output line" {
		t.Fatalf("expected trimmed fallback compact line, got %q", got.CompactLine)
	}
}

func TestDefaultRendererDisplayLabel(t *testing.T) {
	if got := (defaultRenderer{}).DisplayLabel(); got != "TOOL" {
		t.Fatalf("expected default renderer label TOOL, got %q", got)
	}
}

func TestRenderToolPayloadNormalizesEmptyRendererOutput(t *testing.T) {
	RegisterToolRenderer("unit_test_empty_renderer", emptyResultRenderer{})

	payload := "  normalize me  "
	got := renderToolPayload("unit_test_empty_renderer", payload)
	if got.Status != "done" {
		t.Fatalf("expected default status done, got %q", got.Status)
	}
	if got.Summary != "normalize me" {
		t.Fatalf("expected normalized summary, got %q", got.Summary)
	}
	if got.CompactLine != "normalize me" {
		t.Fatalf("expected normalized compact line, got %q", got.CompactLine)
	}
}

func TestReadFileRendererRenderStructuredPayload(t *testing.T) {
	payload := `{"path":"E:/repo/src/main.go","start_line":10,"end_line":42}`

	got := readFileRenderer{}.Render(payload)
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if got.Summary != "Read main.go" {
		t.Fatalf("expected file summary, got %q", got.Summary)
	}
	if got.CompactLine != "main.go (10-42)" {
		t.Fatalf("expected compact range line, got %q", got.CompactLine)
	}
	if len(got.DetailLines) != 2 {
		t.Fatalf("expected 2 detail lines, got %d", len(got.DetailLines))
	}
	if got.DetailLines[0] != "range: 10-42" {
		t.Fatalf("expected range detail, got %q", got.DetailLines[0])
	}
	if !strings.Contains(got.DetailLines[1], "path: E:/repo/src/main.go") {
		t.Fatalf("expected path detail, got %q", got.DetailLines[1])
	}
}

func TestListFilesRendererRenderCountsFilesAndDirs(t *testing.T) {
	payload := `{"items":[{"type":"file"},{"type":"dir"},{"type":"file"},{"type":"dir"}]}`

	got := listFilesRenderer{}.Render(payload)
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if got.Summary != "Read 2 files, listed 2 directories" {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != "2 files, 2 dirs" {
		t.Fatalf("unexpected compact line: %q", got.CompactLine)
	}
}

func TestSearchTextRendererRenderSummarizesMatches(t *testing.T) {
	payload := `{"query":"needle","matches":[{"path":"a.go","line":3,"text":"needle"},{"path":"b.go","line":9,"text":"needle again"}]}`

	got := searchTextRenderer{}.Render(payload)
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if got.Summary != `2 matches for "needle"` {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != got.Summary {
		t.Fatalf("expected compact line to mirror summary, got %q", got.CompactLine)
	}
}

func TestRunShellRendererRenderErrorEnvelope(t *testing.T) {
	payload := `{"ok":false,"error":"permission denied while running command"}`

	got := runShellRenderer{}.Render(payload)
	if got.Status != "error" {
		t.Fatalf("expected error status, got %q", got.Status)
	}
	if !strings.Contains(got.Summary, "permission denied") {
		t.Fatalf("expected error summary to include message, got %q", got.Summary)
	}
	if strings.TrimSpace(got.CompactLine) == "" {
		t.Fatalf("expected compact line for raw payload fallback")
	}
}

func TestRunShellRendererRenderWarnWhenCommandFails(t *testing.T) {
	payload := `{"ok":false,"exit_code":2,"stdout":"line one\nline two","stderr":"warn line\nmore"}`

	got := runShellRenderer{}.Render(payload)
	if got.Status != "warn" {
		t.Fatalf("expected warn status when ok=false, got %q", got.Status)
	}
	if got.Summary != "Shell exited with code 2" {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != "exit code 2" {
		t.Fatalf("unexpected compact line: %q", got.CompactLine)
	}
	if len(got.DetailLines) != 2 {
		t.Fatalf("expected stdout/stderr lines, got %#v", got.DetailLines)
	}
	if !strings.HasPrefix(got.DetailLines[0], "stdout: line one") {
		t.Fatalf("unexpected stdout line: %q", got.DetailLines[0])
	}
	if !strings.HasPrefix(got.DetailLines[1], "stderr: warn line") {
		t.Fatalf("unexpected stderr line: %q", got.DetailLines[1])
	}
}

func TestFileMutationRenderersRenderStructuredPayload(t *testing.T) {
	tests := []struct {
		name          string
		renderer      ToolRenderer
		payload       string
		wantSummary   string
		wantCompact   string
		wantDetailSub string
	}{
		{
			name:          "write_file",
			renderer:      writeFileRenderer{},
			payload:       `{"path":"E:/repo/out.txt","bytes_written":128}`,
			wantSummary:   "Created out.txt",
			wantCompact:   "out.txt",
			wantDetailSub: "wrote 128 bytes",
		},
		{
			name:          "replace_in_file",
			renderer:      replaceInFileRenderer{},
			payload:       `{"path":"E:/repo/main.go","replaced":3,"old_count":1}`,
			wantSummary:   "Updated main.go",
			wantCompact:   "main.go (3 lines)",
			wantDetailSub: "replaced 3 lines",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.renderer.Render(tc.payload)
			if got.Status != "done" {
				t.Fatalf("expected done status, got %q", got.Status)
			}
			if got.Summary != tc.wantSummary {
				t.Fatalf("summary mismatch: got %q want %q", got.Summary, tc.wantSummary)
			}
			if got.CompactLine != tc.wantCompact {
				t.Fatalf("compact mismatch: got %q want %q", got.CompactLine, tc.wantCompact)
			}
			if len(got.DetailLines) == 0 || !strings.Contains(got.DetailLines[0], tc.wantDetailSub) {
				t.Fatalf("expected detail to contain %q, got %#v", tc.wantDetailSub, got.DetailLines)
			}
		})
	}
}

func TestApplyPatchRendererRenderBranches(t *testing.T) {
	ops := make([]map[string]string, 0, 11)
	for i := 1; i <= 11; i++ {
		ops = append(ops, map[string]string{
			"type": "update",
			"path": fmt.Sprintf("E:/repo/src/file%d.go", i),
		})
	}
	rawMany, err := json.Marshal(map[string]any{"operations": ops})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	many := applyPatchRenderer{}.Render(string(rawMany))
	if many.Status != "done" {
		t.Fatalf("expected done status, got %q", many.Status)
	}
	if many.Summary != "Updated 11 files" {
		t.Fatalf("unexpected summary: %q", many.Summary)
	}
	if many.CompactLine != "11 files" {
		t.Fatalf("unexpected compact line: %q", many.CompactLine)
	}
	if len(many.DetailLines) != 11 {
		t.Fatalf("expected 10 operation lines plus ellipsis, got %d", len(many.DetailLines))
	}
	if many.DetailLines[len(many.DetailLines)-1] != "..." {
		t.Fatalf("expected trailing ellipsis for truncated operations, got %#v", many.DetailLines)
	}

	rawSingle, err := json.Marshal(map[string]any{
		"operations": []map[string]string{
			{"type": "update", "path": "E:/repo/src/solo.go"},
		},
	})
	if err != nil {
		t.Fatalf("marshal single payload: %v", err)
	}

	single := applyPatchRenderer{}.Render(string(rawSingle))
	if single.Summary != "Updated 1 files" {
		t.Fatalf("unexpected single summary: %q", single.Summary)
	}
	if single.CompactLine != "solo.go" {
		t.Fatalf("expected compact line to use base name for single file, got %q", single.CompactLine)
	}
}

func TestUpdatePlanRendererRenderLimitsPreviewSteps(t *testing.T) {
	steps := []map[string]string{
		{"title": "collect context", "status": "completed"},
		{"title": "design tests", "status": "in_progress"},
		{"title": "implement tests", "status": "pending"},
		{"title": "run tests", "status": "pending"},
		{"title": "ship", "status": "pending"},
	}
	raw, err := json.Marshal(map[string]any{
		"plan": map[string]any{
			"steps": steps,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	got := updatePlanRenderer{}.Render(string(raw))
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if got.Summary != "Updated plan with 5 step(s)" {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != "5 step(s)" {
		t.Fatalf("unexpected compact line: %q", got.CompactLine)
	}
	if len(got.DetailLines) != 4 {
		t.Fatalf("expected only first 4 steps in details, got %#v", got.DetailLines)
	}
	if !strings.Contains(got.DetailLines[0], "[completed] collect context") {
		t.Fatalf("unexpected first detail line: %q", got.DetailLines[0])
	}
}

func TestWebSearchRendererRenderUsesURLFallbackTitle(t *testing.T) {
	payload := `{"query":"bytemind","results":[{"title":"","url":"https://example.com/a"},{"title":"Result B","url":"https://example.com/b"}]}`

	got := webSearchRenderer{}.Render(payload)
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if got.Summary != `Web search for "bytemind"` {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != `2 results for "bytemind"` {
		t.Fatalf("unexpected compact line: %q", got.CompactLine)
	}
	if len(got.DetailLines) < 3 {
		t.Fatalf("expected count + result previews, got %#v", got.DetailLines)
	}
	if got.DetailLines[0] != "results: 2" {
		t.Fatalf("unexpected first detail line: %q", got.DetailLines[0])
	}
	if !strings.Contains(got.DetailLines[1], "https://example.com/a - https://example.com/a") {
		t.Fatalf("expected URL fallback title in first result line, got %q", got.DetailLines[1])
	}
}

func TestWebFetchRendererRenderIncludesOptionalFields(t *testing.T) {
	payload := `{"url":"https://example.com/docs","status_code":200,"title":"Docs","content":"First line of content","truncated":true}`

	got := webFetchRenderer{}.Render(payload)
	if got.Status != "done" {
		t.Fatalf("expected done status, got %q", got.Status)
	}
	if !strings.HasPrefix(got.Summary, "Fetched https://example.com/docs") {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if got.CompactLine != "https://example.com/docs" {
		t.Fatalf("unexpected compact line: %q", got.CompactLine)
	}
	wantLines := []string{
		"status: HTTP 200",
		"title: Docs",
		"preview: First line of content",
		"content: truncated",
	}
	for _, want := range wantLines {
		found := false
		for _, line := range got.DetailLines {
			if line == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected detail line %q, got %#v", want, got.DetailLines)
		}
	}
}

func TestBuiltinRenderersFallbackOnInvalidJSON(t *testing.T) {
	renderers := []ToolRenderer{
		readFileRenderer{},
		listFilesRenderer{},
		searchTextRenderer{},
		runShellRenderer{},
		writeFileRenderer{},
		replaceInFileRenderer{},
		applyPatchRenderer{},
		updatePlanRenderer{},
		webSearchRenderer{},
		webFetchRenderer{},
	}

	payload := "{"
	for _, renderer := range renderers {
		got := renderer.Render(payload)
		if got.Status != "done" {
			t.Fatalf("expected fallback status done for %T, got %q", renderer, got.Status)
		}
		if got.Summary != payload {
			t.Fatalf("expected fallback summary %q for %T, got %q", payload, renderer, got.Summary)
		}
		if got.CompactLine != payload {
			t.Fatalf("expected fallback compact line %q for %T, got %q", payload, renderer, got.CompactLine)
		}
	}
}
