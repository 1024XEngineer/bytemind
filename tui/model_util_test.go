package tui

import (
	"runtime"
	"testing"
)

// --- toolEntryTitle ---

func TestToolEntryTitleFormatsCorrectly(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"run_shell", "SHELL | run_shell"},
		{"read_file", "READ | read_file"},
		{"list_files", "LIST | list_files"},
		{"write_file", "WRITE | write_file"},
		{"replace_in_file", "EDIT | replace_in_file"},
		{"search_text", "SEARCH | search_text"},
		{"apply_patch", "PATCH | apply_patch"},
		{"web_search", "SEARCH | web_search"},
		{"web_fetch", "FETCH | web_fetch"},
		{"custom_tool", "TOOL | custom_tool"},
		{"", "TOOL | tool"},
	}
	for _, tt := range tests {
		got := toolEntryTitle(tt.name)
		if got != tt.want {
			t.Errorf("toolEntryTitle(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- toolDisplayParts ---

func TestToolDisplayPartsParsesToolCallPrefix(t *testing.T) {
	label, name := toolDisplayParts("Tool Call | run_shell")
	if label != "SHELL" || name != "run_shell" {
		t.Fatalf("expected (SHELL, run_shell), got (%q, %q)", label, name)
	}
}

func TestToolDisplayPartsParsesToolResultPrefix(t *testing.T) {
	label, name := toolDisplayParts("Tool Result | read_file")
	if label != "READ" || name != "read_file" {
		t.Fatalf("expected (READ, read_file), got (%q, %q)", label, name)
	}
}

func TestToolDisplayPartsParsesGenericSeparator(t *testing.T) {
	label, name := toolDisplayParts("SHELL | run_shell")
	if label != "SHELL" || name != "run_shell" {
		t.Fatalf("expected (SHELL, run_shell), got (%q, %q)", label, name)
	}
}

func TestToolDisplayPartsHandlesEmptyTitle(t *testing.T) {
	label, name := toolDisplayParts("")
	if label != "TOOL" || name != "tool" {
		t.Fatalf("expected (TOOL, tool), got (%q, %q)", label, name)
	}
}

func TestToolDisplayPartsHandlesSingleName(t *testing.T) {
	label, name := toolDisplayParts("run_shell")
	if label != "SHELL" || name != "run_shell" {
		t.Fatalf("expected (SHELL, run_shell), got (%q, %q)", label, name)
	}
}

// --- toolDisplayLabel ---

func TestToolDisplayLabelMapsKnownTools(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"list_files", "LIST"},
		{"read_file", "READ"},
		{"search_text", "SEARCH"},
		{"web_search", "SEARCH"},
		{"web_fetch", "FETCH"},
		{"run_shell", "SHELL"},
		{"write_file", "WRITE"},
		{"replace_in_file", "EDIT"},
		{"apply_patch", "PATCH"},
		{"custom_tool", "TOOL"},
		{"", "TOOL"},
	}
	for _, tt := range tests {
		got := toolDisplayLabel(tt.name)
		if got != tt.want {
			t.Errorf("toolDisplayLabel(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- compactDisplayPath ---

func TestCompactDisplayPathShortPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"", ""},
		{"file.go", "file.go"},
		{"src/file.go", "src/file.go"},
		{"a/b/c", "a/b/c"},
	}
	for _, tt := range tests {
		got := compactDisplayPath(tt.path)
		if got != tt.want {
			t.Errorf("compactDisplayPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestCompactDisplayPathLongUnixPath(t *testing.T) {
	got := compactDisplayPath("/home/user/project/src/deep/nested/file.go")
	if runtime.GOOS == "windows" {
		// VolumeName returns "" for unix paths on Windows, so leading "/" is stripped
		if got != "home/.../nested/file.go" {
			t.Fatalf("expected compacted path, got %q", got)
		}
	} else {
		if got != "/home/.../nested/file.go" {
			t.Fatalf("expected compacted path, got %q", got)
		}
	}
}

func TestCompactDisplayPathLongWindowsPath(t *testing.T) {
	got := compactDisplayPath(`C:\Users\dev\project\src\deep\nested\file.go`)
	if runtime.GOOS == "windows" {
		if got != `C:\Users\...\nested\file.go` {
			t.Fatalf("expected compacted windows path, got %q", got)
		}
	} else {
		// On Linux, VolumeName returns "" for Windows-style paths
		if got != `C:\...\nested\file.go` {
			t.Fatalf("expected compacted windows path on linux, got %q", got)
		}
	}
}

// --- joinSummary ---

func TestJoinSummaryNoLines(t *testing.T) {
	got := joinSummary("summary", nil)
	if got != "summary" {
		t.Fatalf("expected \"summary\", got %q", got)
	}
}

func TestJoinSummaryWithLines(t *testing.T) {
	got := joinSummary("summary", []string{"line1", "line2"})
	want := "summary\nline1\nline2"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestJoinSummaryEmptyLines(t *testing.T) {
	got := joinSummary("summary", []string{})
	if got != "summary" {
		t.Fatalf("expected \"summary\", got %q", got)
	}
}

// --- truncateContent ---

func TestTruncateContentShortContent(t *testing.T) {
	lines := truncateContent("line1\nline2\nline3", 5)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestTruncateContentExactLimit(t *testing.T) {
	lines := truncateContent("a\nb\nc", 3)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestTruncateContentOverLimit(t *testing.T) {
	lines := truncateContent("a\nb\nc\nd\ne", 3)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (3 + ellipsis), got %d", len(lines))
	}
	if lines[3] != "..." {
		t.Fatalf("expected \"...\", got %q", lines[3])
	}
}

func TestTruncateContentSingleLine(t *testing.T) {
	lines := truncateContent("hello", 5)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestTruncateContentWindowsLineEndings(t *testing.T) {
	lines := truncateContent("a\r\nb\r\nc", 2)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (2 + ellipsis), got %d", len(lines))
	}
	if lines[2] != "..." {
		t.Fatalf("expected \"...\", got %q", lines[2])
	}
}

// --- summarizeArgs ---

func TestSummarizeArgsEmpty(t *testing.T) {
	if got := summarizeArgs(""); got != "default arguments" {
		t.Fatalf("expected \"default arguments\", got %q", got)
	}
	if got := summarizeArgs("{}"); got != "default arguments" {
		t.Fatalf("expected \"default arguments\", got %q", got)
	}
	if got := summarizeArgs("   "); got != "default arguments" {
		t.Fatalf("expected \"default arguments\", got %q", got)
	}
}

func TestSummarizeArgsNonEmpty(t *testing.T) {
	got := summarizeArgs(`{"path":"file.go","content":"hello"}`)
	if got == "default arguments" {
		t.Fatalf("expected non-default for non-empty args")
	}
	if len(got) == 0 {
		t.Fatalf("expected non-empty summary")
	}
}

// --- assistantToolFollowUp / assistantToolIntro ---

func TestAssistantToolIntroKnownTool(t *testing.T) {
	got := assistantToolIntro("run_shell")
	if got == "" {
		t.Fatalf("expected non-empty intro")
	}
}

func TestAssistantToolIntroEmptyTool(t *testing.T) {
	got := assistantToolIntro("")
	if got == "" {
		t.Fatalf("expected non-empty intro for empty tool")
	}
}

func TestAssistantToolFollowUpWithSummary(t *testing.T) {
	got := assistantToolFollowUp("run_shell", "exit code 0", "done")
	if got == "" {
		t.Fatalf("expected non-empty follow-up")
	}
}

func TestAssistantToolFollowUpEmptySummary(t *testing.T) {
	got := assistantToolFollowUp("run_shell", "", "done")
	if got == "" {
		t.Fatalf("expected non-empty follow-up for empty summary")
	}
}

func TestAssistantToolFollowUpErrorStatus(t *testing.T) {
	got := assistantToolFollowUp("run_shell", "failed", "error")
	if got == "" {
		t.Fatalf("expected non-empty follow-up for error")
	}
}

// --- summarizeTool ---

func TestSummarizeToolListFiles(t *testing.T) {
	payload := `{"root":"/project","items":[{"path":"a.go","type":"file"},{"path":"b.go","type":"file"},{"path":"src","type":"dir"}]}`
	summary, lines, status := summarizeTool("list_files", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	_ = lines
}

func TestSummarizeToolReadFile(t *testing.T) {
	payload := `{"path":"/project/main.go","start_line":1,"end_line":20}`
	summary, lines, status := summarizeTool("read_file", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	if len(lines) == 0 {
		t.Fatalf("expected detail lines")
	}
}

func TestSummarizeToolSearchText(t *testing.T) {
	payload := `{"query":"TODO","matches":[{"path":"a.go","line":5,"text":"// TODO fix"}]}`
	summary, _, status := summarizeTool("search_text", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolRunShell(t *testing.T) {
	payload := `{"ok":true,"exit_code":0,"stdout":"hello world","stderr":""}`
	summary, lines, status := summarizeTool("run_shell", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	_ = lines
}

func TestSummarizeToolRunShellFailed(t *testing.T) {
	payload := `{"ok":false,"exit_code":1,"stdout":"","stderr":"command not found"}`
	_, _, status := summarizeTool("run_shell", payload)
	if status != "warn" {
		t.Fatalf("expected warn for failed shell, got %q", status)
	}
}

func TestSummarizeToolWriteFile(t *testing.T) {
	payload := `{"path":"/project/new.go","bytes_written":1024}`
	summary, _, status := summarizeTool("write_file", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolReplaceInFile(t *testing.T) {
	payload := `{"path":"/project/main.go","replaced":3,"old_count":3}`
	summary, _, status := summarizeTool("replace_in_file", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolApplyPatch(t *testing.T) {
	payload := `{"operations":[{"type":"create","path":"a.go"},{"type":"modify","path":"b.go"}]}`
	summary, _, status := summarizeTool("apply_patch", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolErrorEnvelope(t *testing.T) {
	payload := `{"ok":false,"error":"permission denied"}`
	summary, _, status := summarizeTool("run_shell", payload)
	if status != "error" {
		t.Fatalf("expected error status, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolUnknownTool(t *testing.T) {
	payload := `{"result":"something"}`
	summary, _, status := summarizeTool("custom_tool", payload)
	if status != "done" {
		t.Fatalf("expected done, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary for unknown tool")
	}
}

func TestSummarizeToolMalformedJSON(t *testing.T) {
	summary, _, status := summarizeTool("run_shell", "not json")
	if status != "done" {
		t.Fatalf("expected done fallback, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty fallback summary")
	}
}

// --- stripStreamControlTags ---

func TestStripStreamControlTagsRemovesTurnIntent(t *testing.T) {
	got := stripStreamControlTags("<turn_intent>finalize</turn_intent>actual content")
	if got != "actual content" {
		t.Fatalf("expected \"actual content\", got %q", got)
	}
}

func TestStripStreamControlTagsEmptyInput(t *testing.T) {
	got := stripStreamControlTags("")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestStripStreamControlTagsNoTags(t *testing.T) {
	got := stripStreamControlTags("just text")
	if got != "just text" {
		t.Fatalf("expected \"just text\", got %q", got)
	}
}
