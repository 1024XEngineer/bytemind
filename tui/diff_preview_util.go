package tui

import (
	"fmt"
	"strconv"
)

// diffPreviewLocal mirrors tools.DiffPreview for JSON unmarshaling in the TUI layer.
type diffPreviewLocal struct {
	Files        []diffFileLocal `json:"files"`
	TotalFiles   int             `json:"total_files"`
	TotalAdded   int             `json:"total_added"`
	TotalRemoved int             `json:"total_removed"`
	Truncated    bool            `json:"truncated"`
}

type diffFileLocal struct {
	Path       string         `json:"path"`
	NewPath    string         `json:"new_path,omitempty"`
	ChangeType string         `json:"change_type"`
	Added      int            `json:"added"`
	Removed    int            `json:"removed"`
	Hunks      []diffHunkLocal `json:"hunks"`
	Truncated  bool           `json:"truncated"`
}

type diffHunkLocal struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

func diffHunkPreviewLines(hunks []diffHunkLocal) []string {
	if len(hunks) == 0 {
		return nil
	}
	const maxPreview = 4
	h := hunks[0]
	count := len(h.Lines)
	if count > maxPreview {
		count = maxPreview
	}
	preview := make([]string, 0, count+1)
	for _, line := range h.Lines[:count] {
		preview = append(preview, line)
	}
	if len(h.Lines) > maxPreview {
		preview = append(preview, "  ("+strconv.Itoa(len(h.Lines)-maxPreview)+" more lines)")
	}
	return preview
}

func lineNum(n int) string {
	return fmt.Sprintf("%7d", n)
}

func diffHunkExpandedLines(hunks []diffHunkLocal) []string {
	if len(hunks) == 0 {
		return nil
	}
	lines := make([]string, 0)
	for _, h := range hunks {
		oldLine := h.OldStart
		newLine := h.NewStart
		for _, l := range h.Lines {
			if len(l) < 1 {
				continue
			}
			prefix := l[0]
			content := l[1:]
			switch prefix {
			case ' ':
				lines = append(lines, lineNum(oldLine)+" "+content)
				oldLine++
				newLine++
			case '-':
				lines = append(lines, lineNum(oldLine)+"-"+content)
				oldLine++
			case '+':
				lines = append(lines, lineNum(newLine)+"+"+content)
				newLine++
			}
		}
	}
	return lines
}

func diffExpandedDetailLines(dp diffPreviewLocal) []string {
	if len(dp.Files) == 0 {
		return nil
	}
	lines := make([]string, 0)
	for _, f := range dp.Files {
		// Summary line like Claude CLI: "Added X line(s), removed Y line(s)"
		if f.Added > 0 || f.Removed > 0 {
			addedText := fmt.Sprintf("Added %d line(s), removed %d line(s)", f.Added, f.Removed)
			if len(dp.Files) > 1 {
				addedText = f.ChangeType + " " + f.Path + ": " + addedText
			}
			lines = append(lines, addedText)
		}
		lines = append(lines, diffHunkExpandedLines(f.Hunks)...)
	}
	if dp.Truncated {
		lines = append(lines, "  (diff truncated, ctrl+o to expand)")
	}
	return lines
}
