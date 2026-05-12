package tui

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/charmbracelet/lipgloss"
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
	// FieldsFunc splits on "/" producing leading empty string, so volume is "" and "/" is stripped
	if got != "home/.../nested/file.go" {
		t.Fatalf("expected compacted path, got %q", got)
	}
}

func TestCompactDisplayPathLongWindowsPath(t *testing.T) {
	got := compactDisplayPath(`C:\Users\dev\project\src\deep\nested\file.go`)
	if runtime.GOOS == "windows" {
		if got != `C:\Users\...\nested\file.go` {
			t.Fatalf("expected compacted windows path, got %q", got)
		}
	} else {
		// On Linux, VolumeName returns "" for Windows-style paths so C: is treated as a segment
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

// --- summarizeTool: rate limit / provider error scenarios ---

func TestSummarizeToolRateLimitErrorEnvelope(t *testing.T) {
	payload := `{"ok":false,"error":"provider rate limited: 429 You have reached the rate limit"}`
	summary, _, status := summarizeTool("search_text", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolRateLimitErrorInRunShell(t *testing.T) {
	payload := `{"ok":false,"error":"provider rate limited: 429 Too Many Requests"}`
	summary, _, status := summarizeTool("run_shell", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolRateLimitErrorInReadFile(t *testing.T) {
	payload := `{"ok":false,"error":"Request failed: provider rate limited: 429"}`
	summary, _, status := summarizeTool("read_file", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolRateLimitErrorInWriteFile(t *testing.T) {
	payload := `{"ok":false,"error":"rate limit exceeded"}`
	summary, _, status := summarizeTool("write_file", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolRateLimitErrorInApplyPatch(t *testing.T) {
	payload := `{"ok":false,"error":"429 rate limit"}`
	summary, _, status := summarizeTool("apply_patch", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSummarizeToolTruncatesLongError(t *testing.T) {
	longError := "provider rate limited: 429 You have reached the rate limit. Please wait before making more requests. Your current plan allows 100 requests per minute."
	payload := `{"ok":false,"error":"` + longError + `"}`
	summary, _, status := summarizeTool("run_shell", payload)
	if status != "error" {
		t.Fatalf("expected error, got %q", status)
	}
	if len(summary) > 100 {
		t.Fatalf("expected truncated summary, got length %d: %q", len(summary), summary)
	}
}

// --- aggregateToolGroupStatus ---

func TestAggregateToolGroupStatusAllDone(t *testing.T) {
	group := []chatEntry{
		{Kind: "tool", Status: "done"},
		{Kind: "tool", Status: "done"},
	}
	if got := aggregateToolGroupStatus(group); got != "done" {
		t.Fatalf("expected done, got %q", got)
	}
}

func TestAggregateToolGroupStatusWithError(t *testing.T) {
	group := []chatEntry{
		{Kind: "tool", Status: "done"},
		{Kind: "tool", Status: "error"},
	}
	if got := aggregateToolGroupStatus(group); got != "error" {
		t.Fatalf("expected error, got %q", got)
	}
}

func TestAggregateToolGroupStatusWithRunning(t *testing.T) {
	group := []chatEntry{
		{Kind: "tool", Status: "done"},
		{Kind: "tool", Status: "running"},
	}
	if got := aggregateToolGroupStatus(group); got != "running" {
		t.Fatalf("expected running, got %q", got)
	}
}

func TestAggregateToolGroupStatusWithWarn(t *testing.T) {
	group := []chatEntry{
		{Kind: "tool", Status: "done"},
		{Kind: "tool", Status: "warn"},
	}
	if got := aggregateToolGroupStatus(group); got != "warn" {
		t.Fatalf("expected warn, got %q", got)
	}
}

func TestAggregateToolGroupStatusAllRunning(t *testing.T) {
	group := []chatEntry{
		{Kind: "tool", Status: "running"},
		{Kind: "tool", Status: "running"},
	}
	if got := aggregateToolGroupStatus(group); got != "running" {
		t.Fatalf("expected running, got %q", got)
	}
}

// --- resolveToolRunSectionStyle ---

func TestResolveToolRunSectionStyleDone(t *testing.T) {
	style := resolveToolRunSectionStyle("done")
	if style.GetBorderStyle() == lipgloss.NormalBorder() {
		t.Fatalf("expected styled border for done")
	}
}

func TestResolveToolRunSectionStyleError(t *testing.T) {
	style := resolveToolRunSectionStyle("error")
	if style.GetBorderStyle() == lipgloss.NormalBorder() {
		t.Fatalf("expected styled border for error")
	}
}

func TestResolveToolRunSectionStyleRunning(t *testing.T) {
	style := resolveToolRunSectionStyle("running")
	_ = style // should not panic
}

// --- renderRunSectionGroup ---

func TestRenderRunSectionGroupSingleErrorTool(t *testing.T) {
	item := chatEntry{
		Kind:   "tool",
		Title:  toolEntryTitle("run_shell"),
		Body:   "Request failed: provider rate limited",
		Status: "error",
	}
	rendered := renderRunSectionGroup([]chatEntry{item}, 80, false, true, model{})
	if rendered == "" {
		t.Fatalf("expected non-empty rendering")
	}
}

func TestRenderRunSectionGroupEmpty(t *testing.T) {
	rendered := renderRunSectionGroup(nil, 80, false, true, model{})
	if rendered != "" {
		t.Fatalf("expected empty rendering, got %q", rendered)
	}
}

// --- normalizePlanActionChoiceText ---

func TestNormalizePlanActionChoiceTextStripsNumberedPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1. Start execution", "start execution"},
		{"2. Adjust plan", "adjust plan"},
		{"a. Option A", "option a"},
		{"1) First option", "first option"},
		{"b: Second option", "second option"},
	}
	for _, tt := range tests {
		got := normalizePlanActionChoiceText(tt.input)
		if got != tt.want {
			t.Errorf("normalizePlanActionChoiceText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePlanActionChoiceTextStripsOptionPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"option 1: Start execution", "start execution"},
		{"option a: Choice A", "choice a"},
		{"Option 2. Something", "something"},
	}
	for _, tt := range tests {
		got := normalizePlanActionChoiceText(tt.input)
		if got != tt.want {
			t.Errorf("normalizePlanActionChoiceText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePlanActionChoiceTextStripsBulletPrefix(t *testing.T) {
	got := normalizePlanActionChoiceText("- Start execution")
	if got != "start execution" {
		t.Fatalf("expected \"start execution\", got %q", got)
	}
	got = normalizePlanActionChoiceText("* Adjust plan")
	if got != "adjust plan" {
		t.Fatalf("expected \"adjust plan\", got %q", got)
	}
}

func TestNormalizePlanActionChoiceTextPlainText(t *testing.T) {
	got := normalizePlanActionChoiceText("Start execution")
	if got != "start execution" {
		t.Fatalf("expected \"start execution\", got %q", got)
	}
}

func TestNormalizePlanActionChoiceTextEmpty(t *testing.T) {
	if got := normalizePlanActionChoiceText(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := normalizePlanActionChoiceText("   "); got != "" {
		t.Fatalf("expected empty for whitespace, got %q", got)
	}
}

// --- isBTWCommand ---

func TestIsBTWCommandWithMessage(t *testing.T) {
	if !isBTWCommand("/btw check the tests") {
		t.Fatalf("expected true for /btw with message")
	}
}

func TestIsBTWCommandWithoutMessage(t *testing.T) {
	if !isBTWCommand("/btw") {
		t.Fatalf("expected true for bare /btw")
	}
}

func TestIsBTWCommandEmpty(t *testing.T) {
	if isBTWCommand("") {
		t.Fatalf("expected false for empty input")
	}
	if isBTWCommand("   ") {
		t.Fatalf("expected false for whitespace input")
	}
}

func TestIsBTWCommandNotBTW(t *testing.T) {
	if isBTWCommand("/help") {
		t.Fatalf("expected false for /help")
	}
	if isBTWCommand("btw something") {
		t.Fatalf("expected false for btw without slash")
	}
}

// --- extractBTWText ---

func TestExtractBTWTextSuccess(t *testing.T) {
	got, err := extractBTWText("/btw check the tests")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "check the tests" {
		t.Fatalf("expected \"check the tests\", got %q", got)
	}
}

func TestExtractBTWTextNoMessage(t *testing.T) {
	_, err := extractBTWText("/btw")
	if err == nil {
		t.Fatalf("expected error for bare /btw")
	}
}

func TestExtractBTWTextNotBTW(t *testing.T) {
	_, err := extractBTWText("/help")
	if err == nil {
		t.Fatalf("expected error for non-btw command")
	}
}

func TestExtractBTWTextEmpty(t *testing.T) {
	_, err := extractBTWText("")
	if err == nil {
		t.Fatalf("expected error for empty input")
	}
}

// --- hasPlanActionChoices ---

func TestHasPlanActionChoicesBothPhrases(t *testing.T) {
	text := "Choose an action:\n1. Start execution\n2. Adjust plan"
	if !hasPlanActionChoices(text) {
		t.Fatalf("expected true for text with both phrases")
	}
}

func TestHasPlanActionChoicesOnlyStart(t *testing.T) {
	text := "Ready to start execution"
	if hasPlanActionChoices(text) {
		t.Fatalf("expected false when only start execution present")
	}
}

func TestHasPlanActionChoicesOnlyAdjust(t *testing.T) {
	text := "You can adjust plan first"
	if hasPlanActionChoices(text) {
		t.Fatalf("expected false when only adjust plan present")
	}
}

func TestHasPlanActionChoicesEmpty(t *testing.T) {
	if hasPlanActionChoices("") {
		t.Fatalf("expected false for empty text")
	}
}

// --- latestAssistantMessageText ---

func TestLatestAssistantMessageTextNilSession(t *testing.T) {
	if got := latestAssistantMessageText(nil); got != "" {
		t.Fatalf("expected empty for nil session, got %q", got)
	}
}

func TestLatestAssistantMessageTextNoMessages(t *testing.T) {
	sess := &session.Session{Messages: []llm.Message{}}
	if got := latestAssistantMessageText(sess); got != "" {
		t.Fatalf("expected empty for no messages, got %q", got)
	}
}

func TestLatestAssistantMessageTextOnlyUserMessages(t *testing.T) {
	sess := &session.Session{Messages: []llm.Message{
		llm.NewUserTextMessage("hello"),
	}}
	if got := latestAssistantMessageText(sess); got != "" {
		t.Fatalf("expected empty for no assistant messages, got %q", got)
	}
}

func TestLatestAssistantMessageTextFindsLastAssistant(t *testing.T) {
	sess := &session.Session{Messages: []llm.Message{
		llm.NewUserTextMessage("question 1"),
		llm.NewAssistantTextMessage("answer 1"),
		llm.NewUserTextMessage("question 2"),
		llm.NewAssistantTextMessage("answer 2"),
	}}
	got := latestAssistantMessageText(sess)
	if got != "answer 2" {
		t.Fatalf("expected \"answer 2\", got %q", got)
	}
}

// --- shortID ---

func TestShortIDShort(t *testing.T) {
	if got := shortID("abc"); got != "abc" {
		t.Fatalf("expected \"abc\", got %q", got)
	}
}

func TestShortIDExact12(t *testing.T) {
	id := "123456789012"
	if got := shortID(id); got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestShortIDLong(t *testing.T) {
	id := "12345678901234567890"
	if got := shortID(id); got != "123456789012" {
		t.Fatalf("expected \"123456789012\", got %q", got)
	}
}

// --- formatUserMeta ---

func TestFormatUserMetaWithModel(t *testing.T) {
	at := time.Date(2026, 5, 4, 14, 30, 0, 0, time.UTC)
	got := formatUserMeta("claude-sonnet", at)
	if !strings.Contains(got, "claude-sonnet") {
		t.Fatalf("expected model in output, got %q", got)
	}
	if !strings.Contains(got, "14:30:00") {
		t.Fatalf("expected time in output, got %q", got)
	}
}

func TestFormatUserMetaEmptyModel(t *testing.T) {
	at := time.Date(2026, 5, 4, 14, 30, 0, 0, time.UTC)
	got := formatUserMeta("", at)
	if !strings.Contains(got, "-") {
		t.Fatalf("expected \"-\" for empty model, got %q", got)
	}
}

// --- normalizeKeyName ---

func TestNormalizeKeyNameStripsSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ctrl+v", "ctrl+v"},
		{"Ctrl + V", "ctrl+v"},
		{"page_up", "pageup"},
		{"PgUp", "pgup"},
		{"  Enter  ", "enter"},
		{"space bar", "spacebar"},
		{"some-key", "somekey"},
	}
	for _, tt := range tests {
		got := normalizeKeyName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeKeyName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}


// --- isMeaningfulThinking ---

func TestIsMeaningfulThinkingGenericPrefix(t *testing.T) {
	tests := []struct {
		body   string
		expect bool
	}{
		{"I will call `run_shell` to inspect the relevant context first.", false},
		{"I'll call read_file to check the file.", false},
		{"Let me call search_text.", false},
		{"I have the tool result. Let me organize the next step.", false},
		{"Analyzing the user's request to understand the bug in the login flow.", true},
		{"The error is caused by a nil pointer dereference in handler.go line 42.", true},
	}
	for _, tt := range tests {
		got := isMeaningfulThinking(tt.body, "run_shell")
		if got != tt.expect {
			t.Errorf("isMeaningfulThinking(%q, %q) = %v, want %v", tt.body, "run_shell", got, tt.expect)
		}
	}
}

func TestIsMeaningfulThinkingEmpty(t *testing.T) {
	if isMeaningfulThinking("", "run_shell") {
		t.Fatalf("expected false for empty body")
	}
	if isMeaningfulThinking("   ", "run_shell") {
		t.Fatalf("expected false for whitespace body")
	}
}

// --- preparePlanForContinuation ---

func TestPreparePlanForContinuationNoStructuredPlan(t *testing.T) {
	state := planpkg.State{Phase: planpkg.PhaseExplore}
	_, err := preparePlanForContinuation(state)
	if err == nil {
		t.Fatalf("expected error for no structured plan")
	}
	if !strings.Contains(err.Error(), "no structured plan") {
		t.Fatalf("expected 'no structured plan' error, got %q", err.Error())
	}
}

func TestPreparePlanForContinuationBlockedWithReason(t *testing.T) {
	state := planpkg.State{
		Phase:       planpkg.PhaseBlocked,
		BlockReason: "waiting for API key",
		Steps:       []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
	}
	_, err := preparePlanForContinuation(state)
	if err == nil {
		t.Fatalf("expected error for blocked plan")
	}
	if !strings.Contains(err.Error(), "waiting for API key") {
		t.Fatalf("expected block reason in error, got %q", err.Error())
	}
}

func TestPreparePlanForContinuationBlockedWithoutReason(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseBlocked,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
	}
	_, err := preparePlanForContinuation(state)
	if err == nil {
		t.Fatalf("expected error for blocked plan without reason")
	}
	if !strings.Contains(err.Error(), "cannot continue") {
		t.Fatalf("expected 'cannot continue' error, got %q", err.Error())
	}
}

func TestPreparePlanForContinuationCompleted(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseCompleted,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepCompleted}},
	}
	_, err := preparePlanForContinuation(state)
	if err == nil {
		t.Fatalf("expected error for completed plan")
	}
	if !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("expected 'already completed' error, got %q", err.Error())
	}
}

func TestPreparePlanForContinuationNotConverged(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseDraft,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
	}
	_, err := preparePlanForContinuation(state)
	if err == nil {
		t.Fatalf("expected error for non-converged plan")
	}
	if !strings.Contains(err.Error(), "not converged") {
		t.Fatalf("expected 'not converged' error, got %q", err.Error())
	}
}

func TestPreparePlanForContinuationSuccess(t *testing.T) {
	state := planpkg.State{
		Phase:               planpkg.PhaseConvergeReady,
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
		Steps: []planpkg.Step{
			{Title: "step1", Status: planpkg.StepPending},
			{Title: "step2", Status: planpkg.StepPending},
		},
	}
	result, err := preparePlanForContinuation(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != planpkg.PhaseExecuting {
		t.Fatalf("expected phase executing, got %q", result.Phase)
	}
	// First pending step should be promoted to in_progress
	if planpkg.NormalizeStepStatus(string(result.Steps[0].Status)) != planpkg.StepInProgress {
		t.Fatalf("expected first step to be in_progress, got %q", result.Steps[0].Status)
	}
	if planpkg.NormalizeStepStatus(string(result.Steps[1].Status)) != planpkg.StepPending {
		t.Fatalf("expected second step to remain pending, got %q", result.Steps[1].Status)
	}
}

func TestPreparePlanForContinuationWithCurrentStep(t *testing.T) {
	state := planpkg.State{
		Phase:               planpkg.PhaseConvergeReady,
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
		Steps: []planpkg.Step{
			{Title: "step1", Status: planpkg.StepCompleted},
			{Title: "step2", Status: planpkg.StepInProgress},
		},
	}
	result, err := preparePlanForContinuation(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not change existing in_progress step
	if planpkg.NormalizeStepStatus(string(result.Steps[1].Status)) != planpkg.StepInProgress {
		t.Fatalf("expected step2 to remain in_progress")
	}
}

// --- resolveActiveChoiceSelection ---

func TestResolveActiveChoiceSelectionNoChoice(t *testing.T) {
	state := planpkg.State{}
	_, ok := resolveActiveChoiceSelection("1", state)
	if ok {
		t.Fatalf("expected false for no active choice")
	}
}

func TestResolveActiveChoiceSelectionMatchByNumber(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseClarify,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
		ActiveChoice: &planpkg.ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose layout",
			Options: []planpkg.ChoiceOption{
				{ID: "sidebar", Title: "Sidebar layout", Shortcut: "s"},
				{ID: "stacked", Title: "Stacked layout", Shortcut: "k"},
			},
		},
	}
	action, ok := resolveActiveChoiceSelection("1", state)
	if !ok {
		t.Fatalf("expected match by number")
	}
	if !strings.Contains(action, "layout") || !strings.Contains(action, "sidebar") {
		t.Fatalf("expected action to contain choice and option IDs, got %q", action)
	}
}

func TestResolveActiveChoiceSelectionMatchByShortcut(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseClarify,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
		ActiveChoice: &planpkg.ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose layout",
			Options: []planpkg.ChoiceOption{
				{ID: "sidebar", Title: "Sidebar layout", Shortcut: "s"},
				{ID: "stacked", Title: "Stacked layout", Shortcut: "k"},
			},
		},
	}
	action, ok := resolveActiveChoiceSelection("k", state)
	if !ok {
		t.Fatalf("expected match by shortcut")
	}
	if !strings.Contains(action, "stacked") {
		t.Fatalf("expected stacked option, got %q", action)
	}
}

func TestResolveActiveChoiceSelectionMatchByTitle(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseClarify,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
		ActiveChoice: &planpkg.ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose layout",
			Options: []planpkg.ChoiceOption{
				{ID: "sidebar", Shortcut: "s", Title: "Sidebar layout"},
				{ID: "stacked", Shortcut: "k", Title: "Stacked layout"},
			},
		},
	}
	action, ok := resolveActiveChoiceSelection("Sidebar layout", state)
	if !ok {
		t.Fatalf("expected match by title")
	}
	if !strings.Contains(action, "sidebar") {
		t.Fatalf("expected sidebar option, got %q", action)
	}
}

func TestResolveActiveChoiceSelectionOtherFreeform(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseClarify,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
		ActiveChoice: &planpkg.ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose layout",
			Options: []planpkg.ChoiceOption{
				{ID: "sidebar", Title: "Sidebar"},
				{ID: "custom", Title: "Custom", Freeform: true},
			},
		},
	}
	action, ok := resolveActiveChoiceSelection("other", state)
	if !ok {
		t.Fatalf("expected match for 'other' with freeform option")
	}
	if !strings.Contains(action, "custom") {
		t.Fatalf("expected custom option, got %q", action)
	}
}

func TestResolveActiveChoiceSelectionNoMatch(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseClarify,
		Steps: []planpkg.Step{{Title: "step1", Status: planpkg.StepPending}},
		ActiveChoice: &planpkg.ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose layout",
			Options: []planpkg.ChoiceOption{
				{ID: "sidebar", Title: "Sidebar"},
				{ID: "stacked", Title: "Stacked"},
			},
		},
	}
	_, ok := resolveActiveChoiceSelection("nonexistent option", state)
	if ok {
		t.Fatalf("expected no match")
	}
}

func TestParseMessageCreatedAtSupportsRFC3339Variants(t *testing.T) {
	if ts, ok := parseMessageCreatedAt("2026-05-08T03:04:05.123456Z"); !ok || ts.IsZero() {
		t.Fatalf("expected RFC3339Nano timestamp to parse, got ok=%v ts=%v", ok, ts)
	}
	if ts, ok := parseMessageCreatedAt("2026-05-08T03:04:05Z"); !ok || ts.IsZero() {
		t.Fatalf("expected RFC3339 timestamp to parse, got ok=%v ts=%v", ok, ts)
	}
	if _, ok := parseMessageCreatedAt("2026/05/08 03:04:05"); ok {
		t.Fatalf("expected non-RFC3339 timestamp to fail parsing")
	}
	if _, ok := parseMessageCreatedAt(""); ok {
		t.Fatalf("expected empty timestamp to fail parsing")
	}
}

func TestDelegateSubAgentArgumentHelpers(t *testing.T) {
	callArgs := map[string]string{
		"call-1": `{"agent":"explorer","task":"scan files"}`,
		"call-2": `{"task":"missing agent"}`,
	}

	if got := resolveAgentIDFromArgs(callArgs, "call-1"); got != "explorer" {
		t.Fatalf("expected explicit agent name, got %q", got)
	}
	if got := resolveAgentIDFromArgs(callArgs, "call-2"); got != "subagent" {
		t.Fatalf("expected missing agent to fall back to subagent, got %q", got)
	}
	if got := resolveAgentIDFromArgs(callArgs, "missing"); got != "" {
		t.Fatalf("expected missing call id to return empty agent, got %q", got)
	}

	agent, task := resolveDelegateSubAgentArgs(callArgs, "call-1")
	if agent != "explorer" || task != "scan files" {
		t.Fatalf("expected delegate args to round-trip, got agent=%q task=%q", agent, task)
	}
	agent, task = resolveDelegateSubAgentArgs(callArgs, "missing")
	if agent != "" || task != "" {
		t.Fatalf("expected missing delegate args to return empty values, got agent=%q task=%q", agent, task)
	}
}

func TestResolveFullSubAgentResultGuards(t *testing.T) {
	if got := resolveFullSubAgentResult(llm.Message{}, "call-1"); got != "" {
		t.Fatalf("expected empty result when meta is nil, got %q", got)
	}

	msgWithNonString := llm.Message{
		Meta: llm.MessageMeta{
			"delegate_subagent_result": 123,
		},
	}
	if got := resolveFullSubAgentResult(msgWithNonString, "call-1"); got != "" {
		t.Fatalf("expected non-string meta payload to be ignored, got %q", got)
	}

	msgWithString := llm.Message{
		Meta: llm.MessageMeta{
			"delegate_subagent_result": `{"ok":true}`,
		},
	}
	if got := resolveFullSubAgentResult(msgWithString, "call-1"); got != `{"ok":true}` {
		t.Fatalf("expected string payload passthrough, got %q", got)
	}
}

func TestTruncatePathMiddleKeepsTailContext(t *testing.T) {
	path := "a/very/long/path/for/bytemind/internal/tui/component_conversation.go"
	truncated := truncatePathMiddle(path, 40)
	if !strings.Contains(truncated, "/...component_conversation.go") && !strings.Contains(truncated, "/.../component_conversation.go") {
		t.Fatalf("expected middle truncation to preserve file tail, got %q", truncated)
	}

	short := truncatePathMiddle("tui/model.go", 40)
	if short != "tui/model.go" {
		t.Fatalf("expected short path to remain unchanged, got %q", short)
	}

	tiny := truncatePathMiddle(path, 10)
	if len(tiny) == 0 {
		t.Fatalf("expected tiny limit to still return non-empty compact output")
	}
}
