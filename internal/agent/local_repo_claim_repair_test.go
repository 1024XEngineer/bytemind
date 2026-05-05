package agent

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/plan"
)

func TestBuildLocalRepoClaimRepairInstruction(t *testing.T) {
	reply := llm.Message{
		Role:    llm.RoleAssistant,
		Content: "test reply content",
	}
	evidence := localRepoClaimEvidence{
		LatestUser:             "test user request",
		ReplyPreview:           "test preview",
		ReferencedPaths:        []string{"/path/to/file"},
		WeakSignals:            []string{"signal1"},
		DirectConfirmations:    []string{"confirm1"},
		ImplementationEvidence: []string{"impl1"},
	}

	t.Run("path unverified", func(t *testing.T) {
		result := buildLocalRepoClaimRepairInstruction(
			localRepoClaimRepairPathUnverified,
			reply,
			"latest user",
			1, 3,
			evidence,
		)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "Attempt 1/3") {
			t.Errorf("should contain attempt info, got: %s", result)
		}
		if !strings.Contains(result, "referenced") {
			t.Errorf("should mention referenced path, got: %s", result)
		}
	})

	t.Run("implementation unverified", func(t *testing.T) {
		result := buildLocalRepoClaimRepairInstruction(
			localRepoClaimRepairImplementationUnverified,
			reply,
			"latest user",
			2, 5,
			evidence,
		)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "Attempt 2/5") {
			t.Error("should contain attempt info")
		}
		if !strings.Contains(result, "runnable implementation") {
			t.Error("should mention implementation")
		}
	})

	t.Run("unknown kind returns empty", func(t *testing.T) {
		result := buildLocalRepoClaimRepairInstruction(
			localRepoClaimRepairNone,
			reply,
			"latest user",
			1, 1,
			localRepoClaimEvidence{},
		)
		if result != "" {
			t.Errorf("expected empty for unknown kind, got: %s", result)
		}
	})

	t.Run("empty evidence falls back to new evidence", func(t *testing.T) {
		result := buildLocalRepoClaimRepairInstruction(
			localRepoClaimRepairPathUnverified,
			reply,
			"user input",
			1, 2,
			localRepoClaimEvidence{},
		)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "user input") {
			t.Error("should contain user input")
		}
	})
}

func TestBuildLocalRepoClaimSoftDowngradeAnswer(t *testing.T) {
	reply := llm.Message{
		Role:    llm.RoleAssistant,
		Content: "test reply content",
	}
	evidence := localRepoClaimEvidence{
		LatestUser:             "test user",
		ReplyPreview:           "preview",
		ReferencedPaths:        []string{"/test/path"},
		DirectConfirmations:    []string{"confirm1"},
		WeakSignals:            []string{"weak1"},
		ImplementationEvidence: []string{"impl1"},
	}

	t.Run("path unverified", func(t *testing.T) {
		result := buildLocalRepoClaimSoftDowngradeAnswer(
			localRepoClaimRepairPathUnverified,
			reply,
			"user",
			evidence,
		)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "cannot directly confirm") {
			t.Error("should mention cannot confirm")
		}
	})

	t.Run("implementation unverified", func(t *testing.T) {
		result := buildLocalRepoClaimSoftDowngradeAnswer(
			localRepoClaimRepairImplementationUnverified,
			reply,
			"user",
			evidence,
		)
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !strings.Contains(result, "runnable implementation") {
			t.Error("should mention implementation")
		}
	})

	t.Run("unknown kind uses reply content fallback", func(t *testing.T) {
		result := buildLocalRepoClaimSoftDowngradeAnswer(
			localRepoClaimRepairNone,
			reply,
			"user",
			localRepoClaimEvidence{},
		)
		if result != "test reply content" {
			t.Errorf("expected reply content fallback, got: %s", result)
		}
	})

	t.Run("empty reply fallback", func(t *testing.T) {
		emptyReply := llm.Message{Content: ""}
		result := buildLocalRepoClaimSoftDowngradeAnswer(
			localRepoClaimRepairNone,
			emptyReply,
			"user",
			localRepoClaimEvidence{},
		)
		if !strings.Contains(result, "cannot verify this local repository claim") {
			t.Error("should have default fallback message")
		}
	})
}

func TestExtractRunShellCommand(t *testing.T) {
	t.Run("valid command", func(t *testing.T) {
		args := `{"command": "ls -la"}`
		result := extractRunShellCommand(args)
		if result != "ls -la" {
			t.Errorf("expected 'ls -la', got: %s", result)
		}
	})

	t.Run("empty command", func(t *testing.T) {
		args := `{"command": ""}`
		result := extractRunShellCommand(args)
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		args := `not valid json`
		result := extractRunShellCommand(args)
		if result != "" {
			t.Errorf("expected empty for invalid json, got: %s", result)
		}
	})

	t.Run("missing command field", func(t *testing.T) {
		args := `{"path": "/some/path"}`
		result := extractRunShellCommand(args)
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("command with whitespace", func(t *testing.T) {
		args := `{"command": "  echo hello  "}`
		result := extractRunShellCommand(args)
		if result != "echo hello" {
			t.Errorf("expected trimmed command, got: %s", result)
		}
	})
}

func TestExtractPathArg(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		args := `{"path": "/test/path"}`
		result := extractPathArg(args)
		if result != "/test/path" {
			t.Errorf("expected '/test/path', got: %s", result)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		args := `{"path": ""}`
		result := extractPathArg(args)
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		args := `invalid json`
		result := extractPathArg(args)
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("missing path field", func(t *testing.T) {
		args := `{"command": "ls"}`
		result := extractPathArg(args)
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("path with whitespace gets normalized", func(t *testing.T) {
		args := `{"path": "  /test/path  "}`
		result := extractPathArg(args)
		if result != "/test/path" {
			t.Errorf("expected normalized path, got: %s", result)
		}
	})
}

func TestExtractSearchTextArgs(t *testing.T) {
	t.Run("valid query and path", func(t *testing.T) {
		args := `{"query": "test search", "path": "/search/path"}`
		query, path := extractSearchTextArgs(args)
		if query != "test search" {
			t.Errorf("expected 'test search', got: %s", query)
		}
		if path != "/search/path" {
			t.Errorf("expected '/search/path', got: %s", path)
		}
	})

	t.Run("empty query and path", func(t *testing.T) {
		args := `{"query": "", "path": ""}`
		query, path := extractSearchTextArgs(args)
		if query != "" {
			t.Errorf("expected empty query, got: %s", query)
		}
		if path != "" {
			t.Errorf("expected empty path, got: %s", path)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		args := `not valid json`
		query, path := extractSearchTextArgs(args)
		if query != "" || path != "" {
			t.Errorf("expected empty results for invalid json")
		}
	})

	t.Run("query with whitespace", func(t *testing.T) {
		args := `{"query": "  leading and trailing  "}`
		query, _ := extractSearchTextArgs(args)
		if query != "leading and trailing" {
			t.Errorf("expected trimmed query, got: %s", query)
		}
	})
}

func TestNewLocalRepoClaimEvidence(t *testing.T) {
	t.Run("normal input", func(t *testing.T) {
		evidence := newLocalRepoClaimEvidence("user request", "assistant reply")
		if evidence.LatestUser != "user request" {
			t.Errorf("expected 'user request', got: %s", evidence.LatestUser)
		}
		if evidence.ReplyPreview != "assistant reply" {
			t.Errorf("expected 'assistant reply', got: %s", evidence.ReplyPreview)
		}
	})

	t.Run("empty user uses placeholder", func(t *testing.T) {
		evidence := newLocalRepoClaimEvidence("", "reply")
		if evidence.LatestUser != "(empty user input)" {
			t.Errorf("expected placeholder, got: %s", evidence.LatestUser)
		}
	})

	t.Run("empty reply uses placeholder", func(t *testing.T) {
		evidence := newLocalRepoClaimEvidence("user", "")
		if evidence.ReplyPreview != "(empty assistant text)" {
			t.Errorf("expected placeholder, got: %s", evidence.ReplyPreview)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		evidence := newLocalRepoClaimEvidence("  user  ", "  reply  ")
		if evidence.LatestUser != "user" || evidence.ReplyPreview != "reply" {
			t.Errorf("expected trimmed values, got: %s, %s", evidence.LatestUser, evidence.ReplyPreview)
		}
	})

	t.Run("truncates long reply", func(t *testing.T) {
		longReply := strings.Repeat("x", 300)
		evidence := newLocalRepoClaimEvidence("user", longReply)
		if len(evidence.ReplyPreview) > 240 {
			t.Errorf("expected truncated to 240, got: %d", len(evidence.ReplyPreview))
		}
	})
}

func TestNormalizeLocalRepoPath(t *testing.T) {
	t.Run("normal path", func(t *testing.T) {
		result := normalizeLocalRepoPath("/test/path/file.go")
		if result != "/test/path/file.go" {
			t.Errorf("expected '/test/path/file.go', got: %s", result)
		}
	})

	t.Run("removes quotes", func(t *testing.T) {
		result := normalizeLocalRepoPath("`/test/path`")
		if result != "/test/path" {
			t.Errorf("expected '/test/path', got: %s", result)
		}
	})

	t.Run("converts backslash to forward slash", func(t *testing.T) {
		result := normalizeLocalRepoPath(`C:\test\path`)
		if result != "C:/test/path" {
			t.Errorf("expected 'C:/test/path', got: %s", result)
		}
	})

	t.Run("handles empty input", func(t *testing.T) {
		result := normalizeLocalRepoPath("")
		if result != "" {
			t.Errorf("expected empty, got: %s", result)
		}
	})

	t.Run("removes surrounding whitespace", func(t *testing.T) {
		result := normalizeLocalRepoPath("  /test/path  ")
		if result != "/test/path" {
			t.Errorf("expected '/test/path', got: %s", result)
		}
	})
}

func TestIsDocumentationPath(t *testing.T) {
	t.Run("readme file", func(t *testing.T) {
		if !isDocumentationPath("/path/README.md") {
			t.Error("expected true for README")
		}
	})

	t.Run("readme without extension", func(t *testing.T) {
		if !isDocumentationPath("/path/README") {
			t.Error("expected true for README")
		}
	})

	t.Run("markdown file", func(t *testing.T) {
		if !isDocumentationPath("/path/doc.md") {
			t.Error("expected true for .md")
		}
	})

	t.Run("text file", func(t *testing.T) {
		if !isDocumentationPath("/path/notes.txt") {
			t.Error("expected true for .txt")
		}
	})

	t.Run("rst file", func(t *testing.T) {
		if !isDocumentationPath("/path/doc.rst") {
			t.Error("expected true for .rst")
		}
	})

	t.Run("adoc file", func(t *testing.T) {
		if !isDocumentationPath("/path/doc.adoc") {
			t.Error("expected true for .adoc")
		}
	})

	t.Run("go file is not documentation", func(t *testing.T) {
		if isDocumentationPath("/path/main.go") {
			t.Error("expected false for .go file")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		if !isDocumentationPath("/path/READMETEST.MD") {
			t.Error("expected true for uppercase")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		if isDocumentationPath("") {
			t.Error("expected false for empty")
		}
	})
}

func TestConfirmsReferencedPathByListing(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		if !confirmsReferencedPathByListing("/test/file.go", "/test/file.go") {
			t.Error("expected true for exact match")
		}
	})

	t.Run("parent directory match", func(t *testing.T) {
		if !confirmsReferencedPathByListing("/test/file.go", "/test") {
			t.Error("expected true for parent directory")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if confirmsReferencedPathByListing("/test/file.go", "/other") {
			t.Error("expected false for different path")
		}
	})

	t.Run("empty reference path", func(t *testing.T) {
		if confirmsReferencedPathByListing("", "/test") {
			t.Error("expected false for empty reference")
		}
	})

	t.Run("dot path matches root files", func(t *testing.T) {
		if !confirmsReferencedPathByListing("/file.go", ".") {
			t.Error("expected true for dot path with root file")
		}
	})
}

func TestAppendUniqueEvidenceItem(t *testing.T) {
	t.Run("adds new item", func(t *testing.T) {
		items := []string{"item1", "item2"}
		result := appendUniqueEvidenceItem(items, "item3")
		if len(result) != 3 {
			t.Errorf("expected 3 items, got: %d", len(result))
		}
	})

	t.Run("skips duplicate", func(t *testing.T) {
		items := []string{"item1", "item2"}
		result := appendUniqueEvidenceItem(items, "item1")
		if len(result) != 2 {
			t.Errorf("expected 2 items, got: %d", len(result))
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		items := []string{}
		result := appendUniqueEvidenceItem(items, "  trimmed  ")
		if len(result) != 1 || result[0] != "trimmed" {
			t.Error("expected trimmed item")
		}
	})

	t.Run("skips empty item", func(t *testing.T) {
		items := []string{"item1"}
		result := appendUniqueEvidenceItem(items, "   ")
		if len(result) != 1 {
			t.Error("expected no change for empty item")
		}
	})
}

func TestFormatLocalRepoEvidence(t *testing.T) {
	t.Run("empty items", func(t *testing.T) {
		result := formatLocalRepoEvidence([]string{})
		if result != "(none)" {
			t.Errorf("expected '(none)', got: %s", result)
		}
	})

	t.Run("single item", func(t *testing.T) {
		result := formatLocalRepoEvidence([]string{"item1"})
		if result != "item1" {
			t.Errorf("expected 'item1', got: %s", result)
		}
	})

	t.Run("multiple items", func(t *testing.T) {
		result := formatLocalRepoEvidence([]string{"item1", "item2", "item3"})
		if !strings.Contains(result, "item1") || !strings.Contains(result, "item2") {
			t.Error("expected joined items")
		}
	})
}

func TestLocalRepoEvidenceTraceObserve(t *testing.T) {
	t.Run("search text adds weak signal", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "search_text",
				Arguments: `{"query": "test", "path": "/search"}`,
			},
		}
		trace.observe(call, []string{})
		if len(trace.evidence.WeakSignals) == 0 {
			t.Error("expected weak signal to be added")
		}
	})

	t.Run("list_files adds weak signal", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "list_files",
				Arguments: `{"path": "/test"}`,
			},
		}
		trace.observe(call, []string{})
		if len(trace.evidence.WeakSignals) == 0 {
			t.Error("expected weak signal to be added")
		}
	})

	t.Run("read_file adds direct confirmation", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "read_file",
				Arguments: `{"path": "/test/file.go"}`,
			},
		}
		trace.observe(call, []string{"test/file.go"})
		if len(trace.evidence.DirectConfirmations) == 0 {
			t.Error("expected direct confirmation")
		}
	})

	t.Run("read_file on non-doc adds implementation signal", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "read_file",
				Arguments: `{"path": "/test/main.go"}`,
			},
		}
		trace.observe(call, []string{})
		if !trace.hasImplementationSignal {
			t.Error("expected implementation signal")
		}
	})

	t.Run("read_file on doc does not add implementation signal", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "read_file",
				Arguments: `{"path": "/test/README.md"}`,
			},
		}
		trace.observe(call, []string{})
		if trace.hasImplementationSignal {
			t.Error("expected no implementation signal for doc")
		}
	})

	t.Run("run_shell confirms path", func(t *testing.T) {
		trace := &localRepoEvidenceTrace{
			evidence:       localRepoClaimEvidence{},
			confirmedPaths: map[string]bool{},
		}
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "run_shell",
				Arguments: `{"command": "go run test/main.go"}`,
			},
		}
		trace.observe(call, []string{"test/main.go"})
		t.Logf("confirmed paths: %v", trace.confirmedPaths)
		if trace.confirmedPaths["test/main.go"] != true {
			t.Error("expected path to be confirmed")
		}
	})

	t.Run("nil trace does not panic", func(t *testing.T) {
		var trace *localRepoEvidenceTrace
		call := llm.ToolCall{
			Function: llm.ToolFunctionCall{
				Name:      "read_file",
				Arguments: `{"path": "/test/file.go"}`,
			},
		}
		trace.observe(call, []string{})
	})
}

func TestLocalRepoEvidenceTraceHasDirectConfirmationForAll(t *testing.T) {
	t.Run("empty paths returns true", func(t *testing.T) {
		trace := localRepoEvidenceTrace{
			confirmedPaths: map[string]bool{},
		}
		if !trace.hasDirectConfirmationForAll([]string{}) {
			t.Error("expected true for empty paths")
		}
	})

	t.Run("all confirmed", func(t *testing.T) {
		trace := localRepoEvidenceTrace{
			confirmedPaths: map[string]bool{
				"file1.go": true,
				"file2.go": true,
			},
		}
		if !trace.hasDirectConfirmationForAll([]string{"file1.go", "file2.go"}) {
			t.Error("expected true when all confirmed")
		}
	})

	t.Run("some not confirmed", func(t *testing.T) {
		trace := localRepoEvidenceTrace{
			confirmedPaths: map[string]bool{
				"file1.go": true,
				"file2.go": false,
			},
		}
		if trace.hasDirectConfirmationForAll([]string{"file1.go", "file2.go"}) {
			t.Error("expected false when some not confirmed")
		}
	})
}

func TestExtractLocalRepoPaths(t *testing.T) {
	t.Run("finds paths in text", func(t *testing.T) {
		paths := extractLocalRepoPaths("Check /test/path/file.go and /another/file.go")
		if len(paths) != 2 {
			t.Errorf("expected 2 paths, got: %d", len(paths))
		}
	})

	t.Run("normalizes paths", func(t *testing.T) {
		paths := extractLocalRepoPaths("Check `/test/path`")
		if len(paths) != 1 || paths[0] != "test/path" {
			t.Error("expected normalized path")
		}
	})
}

func TestLooksLikeConcreteLocalRepoClaim(t *testing.T) {
	t.Run("with already token and path", func(t *testing.T) {
		if !looksLikeConcreteLocalRepoClaim("The file already exists at /test/path/main.go", []string{"/test/path/main.go"}) {
			t.Error("expected true")
		}
	})

	t.Run("without path returns false", func(t *testing.T) {
		if looksLikeConcreteLocalRepoClaim("The file already exists", []string{}) {
			t.Error("expected false without path")
		}
	})
}

func TestLooksLikeStrongRepoImplementationClaim(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"already implemented", true},
		{"already has a working solution", true},
		{"can directly run", true},
		{"ready to run", true},
		{"fully working", true},
		{"minimal implementation already exists", true},
		{"implementation is already there", true},
		{"no implementation yet", false},
		{"need to build", false},
	}

	for _, tc := range tests {
		result := looksLikeStrongRepoImplementationClaim(tc.text)
		if result != tc.expected {
			t.Errorf("for %q: expected %v, got %v", tc.text, tc.expected, result)
		}
	}
}

func TestHasUnverifiedLocalRepoQualifier(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"not confirmed yet", true},
		{"unconfirmed", true},
		{"haven't confirmed", true},
		{"have not confirmed", true},
		{"did not confirm", true},
		{"not yet verified", true},
		{"readme mentions", true},
		{"docs mention", true},
		{"confirmed", false},
		{"verified", false},
	}

	for _, tc := range tests {
		result := hasUnverifiedLocalRepoQualifier(tc.text)
		if result != tc.expected {
			t.Errorf("for %q: expected %v, got %v", tc.text, tc.expected, result)
		}
	}
}

func TestEvaluateLocalRepoClaimRepairTurn(t *testing.T) {
	t.Run("plan mode returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModePlan,
			"user",
			llm.Message{Content: "test"},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none for plan mode")
		}
	})

	t.Run("with tool calls returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{
				Content:   "test",
				ToolCalls: []llm.ToolCall{{}},
			},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none when tool calls present")
		}
	})

	t.Run("empty content returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: ""},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none for empty content")
		}
	})

	t.Run("unverified qualifier returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: "The path is not confirmed yet /test/file.go"},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none for unverified qualifier")
		}
	})

	t.Run("no path in text returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: "The file already exists"},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none without path")
		}
	})

	t.Run("concrete claim with path returns path unverified", func(t *testing.T) {
		kind, evidence := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: "The file already exists at /test/file.go"},
			[]llm.Message{{Role: llm.RoleUser, Content: "user input"}},
		)
		if kind != localRepoClaimRepairPathUnverified {
			t.Errorf("expected path unverified, got: %v", kind)
		}
		if len(evidence.ReferencedPaths) == 0 {
			t.Error("expected referenced paths")
		}
	})

	t.Run("no user message returns none", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: "Check /test/file.go"},
			[]llm.Message{},
		)
		if kind != localRepoClaimRepairNone {
			t.Error("expected none without user message")
		}
	})

	t.Run("strong implementation claim returns implementation unverified", func(t *testing.T) {
		kind, _ := evaluateLocalRepoClaimRepairTurn(
			plan.ModeBuild,
			"user",
			llm.Message{Content: "Already has a minimal implementation"},
			[]llm.Message{{Role: llm.RoleUser, Content: "user input"}},
		)
		if kind != localRepoClaimRepairImplementationUnverified {
			t.Errorf("expected implementation unverified, got: %v", kind)
		}
	})
}

func TestInspectLocalRepoEvidence(t *testing.T) {
	t.Run("extracts tool calls from messages", func(t *testing.T) {
		evidence := localRepoClaimEvidence{
			ReferencedPaths: []string{"test/file.go"},
		}
		messages := []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						Function: llm.ToolFunctionCall{
							Name:      "read_file",
							Arguments: `{"path": "/test/file.go"}`,
						},
					},
				},
			},
		}
		trace := inspectLocalRepoEvidence(messages, evidence, []string{"test/file.go"})
		t.Logf("confirmed paths: %v", trace.confirmedPaths)
		if len(trace.confirmedPaths) == 0 {
			t.Error("expected paths in map")
		}
	})

	t.Run("initializes confirmed paths map", func(t *testing.T) {
		evidence := localRepoClaimEvidence{}
		trace := inspectLocalRepoEvidence([]llm.Message{}, evidence, []string{"path1", "path2"})
		t.Logf("confirmed paths: %v", trace.confirmedPaths)
		if len(trace.confirmedPaths) != 2 {
			t.Errorf("expected 2 paths in map, got: %d", len(trace.confirmedPaths))
		}
	})
}
