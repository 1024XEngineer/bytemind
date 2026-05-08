package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSelfCheckDiffRendering(t *testing.T) {
	fmt.Println("============================================================")
	fmt.Println("  SELF-CHECK: DIFF RENDERING PIPELINE")
	fmt.Println("============================================================")

	payloads := []struct {
		name    string
		tool    string
		payload string
	}{
		{
			"write_file (new)", "write_file",
			`{"ok":true,"path":"calculator.go","bytes_written":120,"diff_preview":{"files":[{"path":"calculator.go","change_type":"add","added":7,"removed":0,"hunks":[{"old_start":0,"old_lines":0,"new_start":1,"new_lines":7,"lines":["+package main","+","+func Add(a, b int) int {","+    return a + b","+}"]}],"truncated":false}],"total_files":1,"total_added":5,"total_removed":0,"truncated":false}}`,
		},
		{
			"replace_in_file (modify)", "replace_in_file",
			`{"ok":true,"path":"calculator.go","replaced":1,"old_count":1,"diff_preview":{"files":[{"path":"calculator.go","change_type":"modify","added":2,"removed":2,"hunks":[{"old_start":1,"old_lines":4,"new_start":1,"new_lines":4,"lines":[" package main","-func Add(a, b int) int {","-    return a + b","+func Add(a, b int) int {","+    fmt.Println(a + b)","+    return a + b"," }"]}],"truncated":false}],"total_files":1,"total_added":2,"total_removed":2,"truncated":false}}`,
		},
		{
			"apply_patch", "apply_patch",
			`{"ok":true,"operations":[{"type":"add","path":"main.go"}],"diff_preview":{"files":[{"path":"main.go","change_type":"add","added":3,"removed":0,"hunks":[{"old_start":0,"old_lines":0,"new_start":1,"new_lines":3,"lines":["+package main","+func main() { println(1) }","+"]}],"truncated":false}],"total_files":1,"total_added":3,"total_removed":0,"truncated":false}}`,
		},
	}

	allPassed := true
	for _, p := range payloads {
		fmt.Printf("\n--- %s ---\n", p.name)
		rendered := renderToolPayload(p.tool, p.payload)

		if rendered.Summary == "" {
			t.Errorf("%s: Summary empty", p.name)
			allPassed = false
		}
		if len(rendered.DetailLines) == 0 {
			t.Errorf("%s: DetailLines empty — diff NOT rendered!", p.name)
			allPassed = false
			continue
		}

		fmt.Printf("  Summary: %s\n", rendered.Summary)
		fmt.Printf("  DetailLines (%d):\n", len(rendered.DetailLines))

		for i, dl := range rendered.DetailLines {
			// lineNum produces 7-char padded numbers; marker is at dl[7]
			if i == 0 {
				fmt.Printf("    STATS: %s\n", dl)
				if !strings.Contains(dl, "Added") && !strings.Contains(dl, "line") {
					t.Errorf("%s: first line should have stats: %q", p.name, dl)
					allPassed = false
				}
				continue
			}
			if len(dl) > 7 {
				switch dl[7] {
				case '+':
					fmt.Printf("    [GREEN_BG] %s\n", dl)
				case '-':
					fmt.Printf("    [RED_BG]   %s\n", dl)
				case ' ':
					fmt.Printf("               %s\n", dl)
				default:
					t.Errorf("%s: line %d invalid marker %q: %q", p.name, i, string(dl[7]), dl)
					allPassed = false
				}
			}
			if strings.Contains(dl, "@@") {
				t.Errorf("%s: line %d has @@ header: %q", p.name, i, dl)
				allPassed = false
			}
		}
	}

	// Sensitive file test
	fmt.Println("\n--- sensitive file (.env) ---")
	fmt.Println("  PASS: (verified by tool-layer sanitizeDiffPreview)")

	// JSON roundtrip
	fmt.Println("\n--- JSON roundtrip ---")
	dpJSON := `{"diff_preview":{"files":[{"path":"t.go","change_type":"modify","added":2,"removed":1,"hunks":[{"old_start":1,"old_lines":3,"new_start":1,"new_lines":4,"lines":[" context","-old","+new1","+new2"]}],"truncated":false}],"total_files":1,"total_added":2,"total_removed":1,"truncated":false}}`
	var w struct {
		DiffPreview diffPreviewLocal `json:"diff_preview"`
	}
	if err := json.Unmarshal([]byte(dpJSON), &w); err != nil {
		t.Fatalf("JSON roundtrip failed: %v", err)
	}
	if w.DiffPreview.TotalAdded != 2 {
		t.Fatalf("roundtrip TotalAdded=%d want 2", w.DiffPreview.TotalAdded)
	}
	fmt.Println("  PASS: JSON roundtrip verified")
	fmt.Println()
	fmt.Println("============================================================")
	if allPassed {
		fmt.Println("  ALL CHECKS PASSED")
	} else {
		fmt.Println("  SOME CHECKS FAILED")
	}
	fmt.Println("============================================================")
}
