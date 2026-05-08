package tools

import (
	"path/filepath"
	"strings"
)

// DiffPreview carries structured code change detail for editing tools.
// It is an optional, backward-compatible field added to tool result JSON.
type DiffPreview struct {
	Files        []DiffFile `json:"files"`
	TotalFiles   int        `json:"total_files"`
	TotalAdded   int        `json:"total_added"`
	TotalRemoved int        `json:"total_removed"`
	Truncated    bool       `json:"truncated"`
}

// DiffFile describes changes to a single file.
type DiffFile struct {
	Path       string     `json:"path"`
	NewPath    string     `json:"new_path,omitempty"`
	ChangeType string     `json:"change_type"` // add, delete, modify, move
	Added      int        `json:"added"`
	Removed    int        `json:"removed"`
	Hunks      []DiffHunk `json:"hunks"`
	Truncated  bool       `json:"truncated"`
}

// DiffHunk represents one unified-diff hunk.
type DiffHunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

const (
	diffMaxHunksPerFile  = 20
	diffMaxLinesPerHunk  = 80
	diffMaxTotalLines    = 1200
	diffContextLineCount = 3
)

// sensitivePathPatterns lists filename patterns whose line-level content is hidden.
var sensitivePathPatterns = []string{
	".env",
	"*.pem",
	"id_rsa",
	"id_ed25519",
	"*.key",
	"credentials.json",
	"secrets.yaml",
	"secrets.yml",
}

// isSensitivePath checks whether a file path matches any sensitive pattern.
func isSensitivePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, pat := range sensitivePathPatterns {
		pat = strings.ToLower(pat)
		if strings.Contains(pat, "*") {
			if strings.HasPrefix(pat, "*") && strings.HasSuffix(base, pat[1:]) {
				return true
			}
			if strings.HasSuffix(pat, "*") && strings.HasPrefix(base, pat[:len(pat)-1]) {
				return true
			}
		} else if base == pat {
			return true
		}
	}
	return false
}

// sanitizeDiffPreview clears line-level hunks for sensitive files but keeps stats.
func sanitizeDiffPreview(dp *DiffPreview) {
	if dp == nil {
		return
	}
	for fi := range dp.Files {
		if isSensitivePath(dp.Files[fi].Path) {
			dp.Files[fi].Hunks = nil
		}
	}
}

// truncateDiff enforces hard limits on diff size.
// It mutates hunks in-place and sets the truncated flag when limits are hit.
func truncateDiff(files []DiffFile) (truncated bool) {
	totalLines := 0
	for fi := range files {
		if len(files[fi].Hunks) > diffMaxHunksPerFile {
			files[fi].Hunks = files[fi].Hunks[:diffMaxHunksPerFile]
			files[fi].Truncated = true
			truncated = true
		}
		for hi := range files[fi].Hunks {
			if len(files[fi].Hunks[hi].Lines) > diffMaxLinesPerHunk {
				files[fi].Hunks[hi].Lines = files[fi].Hunks[hi].Lines[:diffMaxLinesPerHunk]
				files[fi].Truncated = true
				truncated = true
			}
			totalLines += len(files[fi].Hunks[hi].Lines)
		}
		if totalLines > diffMaxTotalLines {
			truncated = true
			break
		}
	}
	return truncated
}
