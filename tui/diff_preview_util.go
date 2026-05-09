package tui

import "fmt"

// diffPreviewLocal mirrors tools.DiffPreview for JSON unmarshaling in the TUI layer.
type diffPreviewLocal struct {
	Files        []diffFileLocal `json:"files"`
	TotalFiles   int             `json:"total_files"`
	TotalAdded   int             `json:"total_added"`
	TotalRemoved int             `json:"total_removed"`
	Truncated    bool            `json:"truncated"`
}

type diffFileLocal struct {
	Path       string          `json:"path"`
	NewPath    string          `json:"new_path,omitempty"`
	ChangeType string          `json:"change_type"`
	Added      int             `json:"added"`
	Removed    int             `json:"removed"`
	Hunks      []diffHunkLocal `json:"hunks"`
	Truncated  bool            `json:"truncated"`
}

type diffHunkLocal struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

// diffDetailLine types: prefixed with a control byte so renderDiffDetailLine
// can style each line correctly.
const (
	diffPath   = "\x00" // file path line (cyan)
	diffStats  = "\x01" // stats line (dim gray)
	diffHunkHdr = "\x02" // @@ hunk header (cyan)
)

func lineNumStr(n int) string {
	return fmt.Sprintf("%7d", n)
}

func diffDetailLine(prefix, text string) string {
	return prefix + text
}

// diffExpandedDetailLines generates detail lines for the TUI card.
// Each line is prefixed with a control byte for styling:
//
//	\x00 = file path (toolDiffPathStyle)
//	\x01 = stats (toolDiffStatsStyle)
//	\x02 = hunk header (toolDiffHunkHeaderStyle)
//	+/-/space = diff content lines (toolDiffAddStyle / RemoveStyle / ContextStyle)
func diffExpandedDetailLines(dp diffPreviewLocal) []string {
	if len(dp.Files) == 0 {
		return nil
	}
	lines := make([]string, 0)
	for _, f := range dp.Files {
		// File path line
		pathDisplay := f.ChangeType + " " + f.Path
		if f.NewPath != "" && f.NewPath != f.Path {
			pathDisplay = f.ChangeType + " " + f.Path + " → " + f.NewPath
		}
		lines = append(lines, diffDetailLine(diffPath, pathDisplay))

		// Stats line
		statsText := fmt.Sprintf("+%d -%d", f.Added, f.Removed)
		lines = append(lines, diffDetailLine(diffStats, statsText))

		// Hunk content
		for _, h := range f.Hunks {
			// Hunk header
			hdr := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			lines = append(lines, diffDetailLine(diffHunkHdr, hdr))

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
					lines = append(lines, lineNumStr(oldLine)+"   "+content)
					oldLine++
					newLine++
				case '-':
					lines = append(lines, lineNumStr(oldLine)+" - "+content)
					oldLine++
				case '+':
					lines = append(lines, lineNumStr(newLine)+" + "+content)
					newLine++
				}
			}
		}
	}
	if dp.Truncated {
		lines = append(lines, diffDetailLine(diffStats, "(diff truncated)"))
	}
	return lines
}
