package tui

import (
	"strings"
	"testing"
)

func TestRenderDiffDetailLineAllPaths(t *testing.T) {
	// Path line (0x00)
	result := renderDiffDetailLine("\x00modify calc.go", 80)
	if !strings.Contains(result, "modify calc.go") {
		t.Errorf("path line should contain text, got %q", result)
	}

	// Stats line (0x01)
	result = renderDiffDetailLine("\x01+10 -5", 80)
	if !strings.Contains(result, "+10 -5") {
		t.Errorf("stats line should contain text, got %q", result)
	}

	// Hunk header (0x02)
	result = renderDiffDetailLine("\x02@@ -1,3 +1,4 @@", 80)
	if !strings.Contains(result, "@@") {
		t.Errorf("hunk header should contain @@, got %q", result)
	}

	// Diff content: added line (7-char line num, space, marker, space, code)
	result = renderDiffDetailLine("      1 + package main", 80)
	if result == "      1 + package main" {
		t.Errorf("added line should be styled, got plain text %q", result)
	}

	// Diff content: removed line
	result = renderDiffDetailLine("      2 - func old()", 80)
	if result == "      2 - func old()" {
		t.Errorf("removed line should be styled, got plain text %q", result)
	}

	// Diff content: context line
	result = renderDiffDetailLine("      3   unchanged", 80)
	if result == "      3   unchanged" {
		t.Errorf("context line should be styled, got plain text %q", result)
	}

	// Non-diff line passes through
	result = renderDiffDetailLine("plain text line", 80)
	if result != "plain text line" {
		t.Errorf("non-diff line should pass through unchanged, got %q", result)
	}

	// Empty line
	result = renderDiffDetailLine("", 80)
	if result != "" {
		t.Errorf("empty line should be empty, got %q", result)
	}

	// Short line (< 10 chars) passes through
	result = renderDiffDetailLine("  short", 80)
	if result != "  short" {
		t.Errorf("short line should pass through, got %q", result)
	}

	// Padding to width
	result = renderDiffDetailLine("      1 + pkg", 80)
	if len(result) < 80 {
		t.Errorf("line should be padded to >= width, got len=%d, content=%q", len(result), result)
	}
}

func TestDiffExpandedDetailLinesMultiFile(t *testing.T) {
	dp := diffPreviewLocal{
		Files: []diffFileLocal{
			{
				Path:       "a.go",
				ChangeType: "modify",
				Added:      1,
				Removed:    1,
				Hunks: []diffHunkLocal{{
					OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
					Lines: []string{"-old", "+new"},
				}},
			},
			{
				Path:       "b.go",
				ChangeType: "add",
				Added:      2,
				Removed:    0,
				Hunks: []diffHunkLocal{{
					OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2,
					Lines: []string{"+line1", "+line2"},
				}},
			},
		},
		TotalFiles:   2,
		TotalAdded:   3,
		TotalRemoved: 1,
	}

	lines := diffExpandedDetailLines(dp)
	if len(lines) < 6 {
		t.Fatalf("expected at least 6 lines for 2 files, got %d", len(lines))
	}
	if lines[0][0] != 0x00 {
		t.Errorf("first line should be path")
	}
	if lines[1][0] != 0x01 {
		t.Errorf("second line should be stats")
	}
}

func TestDiffExpandedDetailLinesTruncated(t *testing.T) {
	dp := diffPreviewLocal{
		Files: []diffFileLocal{{
			Path: "x.go", ChangeType: "add", Added: 1,
			Hunks: []diffHunkLocal{{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 1, Lines: []string{"+x"}}},
		}},
		Truncated: true,
	}
	lines := diffExpandedDetailLines(dp)
	found := false
	for _, l := range lines {
		if l[0] == 0x01 && strings.Contains(l[1:], "truncated") {
			found = true
		}
	}
	if !found {
		t.Errorf("truncated dp should have truncated notice in stats")
	}
}

func TestDiffExpandedDetailLinesEmpty(t *testing.T) {
	if diffExpandedDetailLines(diffPreviewLocal{}) != nil {
		t.Error("empty dp should return nil")
	}
}

func TestRenderToolPayloadDiffPaths(t *testing.T) {
	// write_file with diff_preview
	payload := `{"ok":true,"path":"calc.go","bytes_written":50,"diff_preview":{"files":[{"path":"calc.go","change_type":"add","added":3,"removed":0,"hunks":[{"old_start":0,"old_lines":0,"new_start":1,"new_lines":3,"lines":["+p1","+p2","+p3"]}],"truncated":false}],"total_files":1,"total_added":3,"total_removed":0,"truncated":false}}`
	r := renderToolPayload("write_file", payload)
	if len(r.DetailLines) < 4 {
		t.Errorf("write_file with diff should have detail lines, got %d", len(r.DetailLines))
	}

	// replace_in_file with diff_preview
	payload2 := `{"ok":true,"path":"calc.go","replaced":1,"old_count":1,"diff_preview":{"files":[{"path":"calc.go","change_type":"modify","added":1,"removed":1,"hunks":[{"old_start":1,"old_lines":2,"new_start":1,"new_lines":2,"lines":["-old","+new"]}],"truncated":false}],"total_files":1,"total_added":1,"total_removed":1,"truncated":false}}`
	r2 := renderToolPayload("replace_in_file", payload2)
	if len(r2.DetailLines) < 4 {
		t.Errorf("replace_in_file with diff should have detail lines, got %d", len(r2.DetailLines))
	}

	// apply_patch with diff_preview
	payload3 := `{"ok":true,"operations":[{"type":"add","path":"x.go"}],"diff_preview":{"files":[{"path":"x.go","change_type":"add","added":2,"removed":0,"hunks":[{"old_start":0,"old_lines":0,"new_start":1,"new_lines":2,"lines":["+a","+b"]}],"truncated":false}],"total_files":1,"total_added":2,"total_removed":0,"truncated":false}}`
	r3 := renderToolPayload("apply_patch", payload3)
	if len(r3.DetailLines) < 4 {
		t.Errorf("apply_patch with diff should have detail lines, got %d", len(r3.DetailLines))
	}

	// tool without diff_preview (fallback)
	payload4 := `{"ok":true,"path":"x.go","bytes_written":10}`
	r4 := renderToolPayload("write_file", payload4)
	if len(r4.DetailLines) == 0 {
		t.Errorf("write_file without diff should still have detail lines")
	}
}

func TestRenderDiffDetailLineEdgeCases(t *testing.T) {
	// Line exactly at minimum length (10 chars)
	result := renderDiffDetailLine("      1   x", 80)
	if len(result) < 80 {
		t.Errorf("min length diff line should be padded, got len=%d", len(result))
	}

	// Line with marker at wrong position (not at 8)
	result2 := renderDiffDetailLine("12345678abc", 80)
	if result2 != "12345678abc" {
		t.Errorf("line without valid marker should pass through")
	}

	// Very long line gets padded
	long := "      1 + " + strings.Repeat("x", 200)
	result3 := renderDiffDetailLine(long, 80)
	if len(result3) < 80 {
		t.Errorf("long line should not be shortened below width")
	}
}
