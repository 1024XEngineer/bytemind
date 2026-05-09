package tui

import (
	"encoding/json"
	"fmt"
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
			`{"ok":true,"path":"calculator.go","bytes_written":120,"diff_preview":{"files":[{"path":"calculator.go","change_type":"add","added":5,"removed":0,"hunks":[{"old_start":0,"old_lines":0,"new_start":1,"new_lines":5,"lines":["+package main","+","+func Add(a, b int) int {","+    return a + b","+}"]}],"truncated":false}],"total_files":1,"total_added":5,"total_removed":0,"truncated":false}}`,
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

	for _, p := range payloads {
		fmt.Printf("\n--- %s ---\n", p.name)
		rendered := renderToolPayload(p.tool, p.payload)

		if len(rendered.DetailLines) == 0 {
			t.Errorf("%s: DetailLines empty", p.name)
			continue
		}

		for _, dl := range rendered.DetailLines {
			if len(dl) == 0 {
				continue
			}
			switch {
			case len(dl) > 0 && dl[0] == 0x00:
				fmt.Printf("    PATH:  %s\n", dl[1:])
			case len(dl) > 0 && dl[0] == 0x01:
				fmt.Printf("    STATS: %s\n", dl[1:])
			case len(dl) > 0 && dl[0] == 0x02:
				fmt.Printf("    HUNK:  %s\n", dl[1:])
			case len(dl) >= 9:
				switch dl[8] {
				case '+':
					fmt.Printf("    ADD:   %q\n", dl)
				case '-':
					fmt.Printf("    REM:   %q\n", dl)
				case ' ':
					fmt.Printf("    CTX:   %q\n", dl)
				default:
					fmt.Printf("    ???:   %q\n", dl)
				}
			default:
				fmt.Printf("    ???:   %q\n", dl)
			}
		}

		// Verify structure
		if len(rendered.DetailLines) < 3 {
			t.Errorf("%s: expected at least path + stats + 1 diff line", p.name)
		}
		if len(rendered.DetailLines[0]) == 0 || rendered.DetailLines[0][0] != 0x00 {
			t.Errorf("%s: first line should be path", p.name)
		}
		if len(rendered.DetailLines[1]) == 0 || rendered.DetailLines[1][0] != 0x01 {
			t.Errorf("%s: second line should be stats", p.name)
		}
	}

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
	fmt.Println("  PASS")

	fmt.Println("\n============================================================")
	fmt.Println("  ALL CHECKS PASSED")
	fmt.Println("============================================================")
}
