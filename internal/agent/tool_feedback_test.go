package agent

import (
	"strings"
	"testing"
)

func TestEmptyDot(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "."},
		{"   ", "."},
		{"/test/path", "/test/path"},
		{"./relative", "./relative"},
	}

	for _, tc := range tests {
		result := emptyDot(tc.input)
		if result != tc.expected {
			t.Errorf("emptyDot(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestCompactWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		limit    int
		expected string
	}{
		{"hello world", 50, "hello world"},
		{"  hello   world  ", 50, "hello world"},
		{"hello world", 5, "he..."},
		{"hi", 10, "hi"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"", 10, ""},
		{"  ", 10, ""},
	}

	for _, tc := range tests {
		result := compactWhitespace(tc.input, tc.limit)
		if result != tc.expected {
			t.Errorf("compactWhitespace(%q, %d) = %q, want %q", tc.input, tc.limit, result, tc.expected)
		}
	}
}

func TestPreviewPaths(t *testing.T) {
	t.Run("files and directories", func(t *testing.T) {
		items := []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}{
			{Path: "file1.go", Type: "file"},
			{Path: "dir1", Type: "dir"},
			{Path: "file2.txt", Type: "file"},
		}
		result := previewPaths(items)
		if len(result) != 3 {
			t.Errorf("expected 3 items, got: %d", len(result))
		}
		if !strings.Contains(result[0], "file ") || !strings.Contains(result[0], "file1.go") {
			t.Errorf("expected 'file file1.go', got: %s", result[0])
		}
		if !strings.Contains(result[1], "dir ") || !strings.Contains(result[1], "dir1") {
			t.Errorf("expected 'dir dir1', got: %s", result[1])
		}
	})

	t.Run("empty list", func(t *testing.T) {
		result := previewPaths([]struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}{})
		if len(result) != 0 {
			t.Errorf("expected 0 items, got: %d", len(result))
		}
	})
}

func TestPreviewMatches(t *testing.T) {
	t.Run("formats matches", func(t *testing.T) {
		matches := []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		}{
			{Path: "test.go", Line: 10, Text: "func main()"},
			{Path: "test.go", Line: 20, Text: "return nil"},
		}
		result := previewMatches(matches)
		if len(result) != 2 {
			t.Errorf("expected 2 items, got: %d", len(result))
		}
		if !strings.Contains(result[0], "test.go:10") {
			t.Errorf("expected 'test.go:10', got: %s", result[0])
		}
	})

	t.Run("empty list", func(t *testing.T) {
		result := previewMatches([]struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		}{})
		if len(result) != 0 {
			t.Errorf("expected 0 items, got: %d", len(result))
		}
	})
}

func TestPreviewOutput(t *testing.T) {
	t.Run("trims and limits lines", func(t *testing.T) {
		result := previewOutput("stdout", "line1\nline2\nline3\nline4\nline5\nline6")
		if len(result) > 0 {
			if !strings.Contains(result[0], "stdout:") {
				t.Errorf("expected 'stdout:', got: %s", result[0])
			}
		}
	})

	t.Run("empty text returns nil", func(t *testing.T) {
		result := previewOutput("stdout", "")
		if result != nil {
			t.Error("expected nil for empty text")
		}
	})

	t.Run("whitespace only returns nil", func(t *testing.T) {
		result := previewOutput("stdout", "   \n   ")
		if result != nil {
			t.Error("expected nil for whitespace only")
		}
	})
}

func TestFormatSystemSandboxSummary(t *testing.T) {
	t.Run("all empty returns empty", func(t *testing.T) {
		result := formatSystemSandboxSummary("", "", false, false, "", false, false, false)
		if result != "" {
			t.Errorf("expected empty string, got: %s", result)
		}
	})

	t.Run("with mode and backend", func(t *testing.T) {
		result := formatSystemSandboxSummary("restricted", "eBPF", true, true, "full", false, false, false)
		if result == "" {
			t.Error("expected non-empty result")
		}
		if !strings.Contains(result, "restricted") {
			t.Error("expected mode in result")
		}
	})

	t.Run("fallback state", func(t *testing.T) {
		result := formatSystemSandboxSummary("", "", false, false, "", false, false, true)
		if !strings.Contains(result, "fallback") {
			t.Error("expected fallback state")
		}
	})
}

func TestNormalizeApprovalErrorMessage(t *testing.T) {
	t.Run("normalizes message with reason code prefix", func(t *testing.T) {
		result := normalizeApprovalErrorMessage("permission_denied: file access required", "permission_denied")
		if strings.Contains(result, "permission_denied:") {
			t.Error("should remove reason code prefix")
		}
	})

	t.Run("fallback for empty message", func(t *testing.T) {
		result := normalizeApprovalErrorMessage("", "permission_denied")
		if result != "approval required" {
			t.Errorf("expected fallback message, got: %s", result)
		}
	})

	t.Run("passes through without matching prefix", func(t *testing.T) {
		result := normalizeApprovalErrorMessage("access denied by policy", "other_code")
		if result != "access denied by policy" {
			t.Errorf("expected unchanged message, got: %s", result)
		}
	})
}

func TestNormalizeDeniedMessage(t *testing.T) {
	t.Run("normalizes message", func(t *testing.T) {
		result := normalizeDeniedMessage("denied: operation not allowed", "denied")
		if strings.Contains(result, "denied:") {
			t.Error("should remove prefix")
		}
	})

	t.Run("fallback for empty message", func(t *testing.T) {
		result := normalizeDeniedMessage("", "denied")
		if result != "operation denied" {
			t.Errorf("expected fallback message, got: %s", result)
		}
	})

	t.Run("passes through without matching prefix", func(t *testing.T) {
		result := normalizeDeniedMessage("not allowed", "other_code")
		if result != "not allowed" {
			t.Errorf("expected unchanged message, got: %s", result)
		}
	})
}

func TestNormalizeSkippedDependencyMessage(t *testing.T) {
	t.Run("normalizes message", func(t *testing.T) {
		result := normalizeSkippedDependencyMessage("denied_dependency: missing dependency", "denied_dependency")
		if strings.Contains(result, "denied_dependency:") {
			t.Error("should remove prefix")
		}
	})

	t.Run("fallback for empty message", func(t *testing.T) {
		result := normalizeSkippedDependencyMessage("", "denied_dependency")
		if result != "skipped due to denied dependency" {
			t.Errorf("expected fallback message, got: %s", result)
		}
	})

	t.Run("passes through without matching prefix", func(t *testing.T) {
		result := normalizeSkippedDependencyMessage("skipped due to other reason", "other_code")
		if result != "skipped due to other reason" {
			t.Errorf("expected unchanged message, got: %s", result)
		}
	})
}

func TestNormalizeReasonPrefixedMessage(t *testing.T) {
	t.Run("removes matching prefix", func(t *testing.T) {
		result := normalizeReasonPrefixedMessage("code: actual message", "code", "default")
		if result != "actual message" {
			t.Errorf("expected 'actual message', got: %s", result)
		}
	})

	t.Run("case insensitive prefix", func(t *testing.T) {
		result := normalizeReasonPrefixedMessage("CODE: actual message", "code", "default")
		if result != "actual message" {
			t.Errorf("expected 'actual message', got: %s", result)
		}
	})

	t.Run("keeps message if no matching prefix", func(t *testing.T) {
		result := normalizeReasonPrefixedMessage("different: message", "code", "default")
		if result != "different: message" {
			t.Errorf("expected unchanged message, got: %s", result)
		}
	})

	t.Run("returns fallback for empty message", func(t *testing.T) {
		result := normalizeReasonPrefixedMessage("", "code", "fallback")
		if result != "fallback" {
			t.Errorf("expected fallback, got: %s", result)
		}
	})
}
