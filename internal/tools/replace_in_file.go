package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type ReplaceInFileTool struct{}

func (ReplaceInFileTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "replace_in_file",
			Description: "Replace exact text in a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path from workspace or absolute path inside workspace/writable_roots.",
					},
					"old": map[string]any{
						"type":        "string",
						"description": "Existing text to replace.",
					},
					"new": map[string]any{
						"type":        "string",
						"description": "Replacement text.",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "Replace all matches instead of only the first.",
					},
				},
				"required": []string{"path", "old", "new"},
			},
		},
	}
}

func (ReplaceInFileTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Path       string `json:"path"`
		Old        string `json:"old"`
		New        string `json:"new"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	path, err := resolvePath(execCtx.Workspace, args.Path, writableRootsFromExecContext(execCtx)...)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, args.Old)
	if count == 0 {
		return "", errors.New("target text not found")
	}

	updated := content
	replaced := 1
	if args.ReplaceAll {
		updated = strings.ReplaceAll(content, args.Old, args.New)
		replaced = count
	} else {
		updated = strings.Replace(content, args.Old, args.New, 1)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}

	var result map[string]any
	relPath := filepath.ToSlash(mustRel(execCtx.Workspace, path))
	if diffPreview := buildReplaceDiff(content, args.Old, args.New, args.ReplaceAll, relPath); diffPreview != nil {
		result = map[string]any{
			"ok":           true,
			"path":         relPath,
			"replaced":     replaced,
			"old_count":    count,
			"diff_preview": diffPreview,
		}
	} else {
		result = map[string]any{
			"ok":        true,
			"path":      relPath,
			"replaced":  replaced,
			"old_count": count,
		}
	}
	return toJSON(result)
}

func buildReplaceDiff(original, oldStr, newStr string, replaceAll bool, path string) *DiffPreview {
	origLines := strings.Split(original, "\n")
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")
	if len(oldLines) == 0 {
		return nil
	}
	positions := findLineMatches(origLines, oldLines)
	if len(positions) == 0 {
		return nil
	}

	maxShow := len(positions)
	if replaceAll && maxShow > 10 {
		maxShow = 10
	}

	totalAdded := len(newLines) * len(positions)
	totalRemoved := len(oldLines) * len(positions)

	hunks := make([]DiffHunk, 0, maxShow)
	for i := 0; i < maxShow; i++ {
		pos := positions[i]
		ctxStart := pos - diffContextLineCount
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd := pos + len(oldLines) + diffContextLineCount
		if ctxEnd > len(origLines) {
			ctxEnd = len(origLines)
		}

		hunkLines := make([]string, 0, ctxEnd-ctxStart+len(newLines))
		for j := ctxStart; j < pos; j++ {
			hunkLines = append(hunkLines, " "+origLines[j])
		}
		for _, l := range oldLines {
			hunkLines = append(hunkLines, "-"+l)
		}
		for _, l := range newLines {
			hunkLines = append(hunkLines, "+"+l)
		}
		for j := pos + len(oldLines); j < ctxEnd; j++ {
			hunkLines = append(hunkLines, " "+origLines[j])
		}

		hunks = append(hunks, DiffHunk{
			OldStart: ctxStart + 1,
			OldLines: ctxEnd - ctxStart,
			NewStart: ctxStart + 1,
			NewLines: ctxEnd - ctxStart - len(oldLines) + len(newLines),
			Lines:    hunkLines,
		})
	}

	truncated := len(positions) > maxShow
	for hi := range hunks {
		if len(hunks[hi].Lines) > diffMaxLinesPerHunk {
			hunks[hi].Lines = hunks[hi].Lines[:diffMaxLinesPerHunk]
			truncated = true
		}
	}
	dp := &DiffPreview{
		Files: []DiffFile{{
			Path:       path,
			ChangeType: "modify",
			Added:      totalAdded,
			Removed:    totalRemoved,
			Hunks:      hunks,
			Truncated:  truncated,
		}},
		TotalFiles:   1,
		TotalAdded:   totalAdded,
		TotalRemoved: totalRemoved,
		Truncated:    truncated,
	}
	sanitizeDiffPreview(dp)
	return dp
}

func findLineMatches(lines, pattern []string) []int {
	if len(pattern) == 0 || len(pattern) > len(lines) {
		return nil
	}
	matches := make([]int, 0, 4)
	for i := 0; i <= len(lines)-len(pattern); i++ {
		match := true
		for j := range pattern {
			if lines[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	return matches
}
