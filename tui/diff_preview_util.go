package tui

import "strconv"

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

func diffHunkExpandedLines(hunks []diffHunkLocal) []string {
	if len(hunks) == 0 {
		return nil
	}
	lines := make([]string, 0)
	for hi, h := range hunks {
		if hi > 0 {
			lines = append(lines, "  ---")
		}
		if h.OldStart > 0 || h.NewStart > 0 {
			lines = append(lines,
				"  @@"+" -"+strconv.Itoa(h.OldStart)+","+strconv.Itoa(h.OldLines)+
					" +"+strconv.Itoa(h.NewStart)+","+strconv.Itoa(h.NewLines)+" @@")
		}
		for _, line := range h.Lines {
			lines = append(lines, line)
		}
	}
	return lines
}

func diffExpandedDetailLines(dp diffPreviewLocal) []string {
	if len(dp.Files) == 0 {
		return nil
	}
	lines := make([]string, 0)
	for fi, f := range dp.Files {
		if fi > 0 {
			lines = append(lines, "  ---")
		}
		if fi == 0 && len(dp.Files) > 1 {
			lines = append(lines, f.ChangeType+" "+f.Path+"  +"+strconv.Itoa(f.Added)+" -"+strconv.Itoa(f.Removed))
		}
		lines = append(lines, diffHunkExpandedLines(f.Hunks)...)
	}
	if dp.Truncated {
		lines = append(lines, "  (diff truncated)")
	}
	return lines
}
