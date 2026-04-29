package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type ApplyPatchTool struct{}

type patchLine struct {
	Prefix byte
	Text   string
}

type patchRange struct {
	Start int
	Count int
}

type patchHunk struct {
	Header    string
	HasHeader bool
	OldRange  patchRange
	NewRange  patchRange
	Lines     []patchLine
}

var unifiedHunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?: .*)?$`)

func (ApplyPatchTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "apply_patch",
			Description: "Apply a structured multi-file patch using Begin Patch / Update File / Add File / Delete File syntax.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patch": map[string]any{
						"type":        "string",
						"description": "Patch text using the apply_patch format.",
					},
				},
				"required": []string{"patch"},
			},
		},
	}
}

func (ApplyPatchTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	patchLineEnding := detectLineEnding(args.Patch)
	patchText := normalizePatchText(args.Patch)
	if patchText == "" {
		return "", errors.New("patch is required")
	}

	lines := strings.Split(patchText, "\n")
	if len(lines) < 2 || lines[0] != "*** Begin Patch" {
		return "", errors.New("patch must start with *** Begin Patch")
	}

	i := 1
	operations := make([]map[string]any, 0, 4)
	for i < len(lines) {
		line := lines[i]
		if line == "*** End Patch" {
			return toJSON(map[string]any{
				"ok":         true,
				"operations": operations,
			})
		}

		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			i++
			contentLines := make([]string, 0, 32)
			for i < len(lines) && !strings.HasPrefix(lines[i], "*** ") {
				if !strings.HasPrefix(lines[i], "+") {
					return "", fmt.Errorf("add file line must start with +: %q", lines[i])
				}
				contentLines = append(contentLines, strings.TrimPrefix(lines[i], "+"))
				i++
			}
			resolved, err := resolvePath(execCtx.Workspace, path, writableRootsFromExecContext(execCtx)...)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
				return "", err
			}
			content := joinLines(contentLines, true, patchLineEnding)
			if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
				return "", err
			}
			operations = append(operations, map[string]any{"type": "add", "path": filepath.ToSlash(mustRel(execCtx.Workspace, resolved))})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			resolved, err := resolvePath(execCtx.Workspace, path, writableRootsFromExecContext(execCtx)...)
			if err != nil {
				return "", err
			}
			if err := os.Remove(resolved); err != nil {
				return "", err
			}
			operations = append(operations, map[string]any{"type": "delete", "path": filepath.ToSlash(mustRel(execCtx.Workspace, resolved))})
			i++
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			oldPath, err := resolvePath(execCtx.Workspace, path, writableRootsFromExecContext(execCtx)...)
			if err != nil {
				return "", err
			}
			i++
			newPath := oldPath
			if i < len(lines) && strings.HasPrefix(lines[i], "*** Move to: ") {
				newPath, err = resolvePath(execCtx.Workspace, strings.TrimSpace(strings.TrimPrefix(lines[i], "*** Move to: ")), writableRootsFromExecContext(execCtx)...)
				if err != nil {
					return "", err
				}
				i++
			}
			chunkLines := make([]string, 0, 64)
			for i < len(lines) && !strings.HasPrefix(lines[i], "*** ") {
				chunkLines = append(chunkLines, lines[i])
				i++
			}
			original, err := os.ReadFile(oldPath)
			if err != nil {
				return "", err
			}
			updated, err := applyStructuredPatch(string(original), chunkLines)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(newPath, []byte(updated), 0o644); err != nil {
				return "", err
			}
			if newPath != oldPath {
				if err := os.Remove(oldPath); err != nil {
					return "", err
				}
			}
			operations = append(operations, map[string]any{
				"type":     "update",
				"path":     filepath.ToSlash(mustRel(execCtx.Workspace, oldPath)),
				"new_path": filepath.ToSlash(mustRel(execCtx.Workspace, newPath)),
			})
		default:
			return "", fmt.Errorf("unsupported patch header: %q", line)
		}
	}

	return "", errors.New("patch missing *** End Patch")
}

func applyStructuredPatch(original string, chunkLines []string) (string, error) {
	hunks, err := parsePatchHunks(chunkLines)
	if err != nil {
		return "", err
	}
	if len(hunks) == 0 {
		return original, nil
	}

	originalLines, trailingNewline, lineEnding := splitLines(original)
	result := make([]string, 0, len(originalLines))
	cursor := 0

	for _, hunk := range hunks {
		oldSeq, newSeq := hunkSequences(hunk)
		if err := validateHunk(hunk, oldSeq, newSeq); err != nil {
			return "", err
		}
		if len(oldSeq) == 0 {
			return "", errors.New("patch hunk without context or removed lines is ambiguous")
		}

		matchAt, err := locateHunk(originalLines, oldSeq, cursor, hunk)
		if err != nil {
			return "", err
		}
		result = append(result, originalLines[cursor:matchAt]...)
		result = append(result, newSeq...)
		cursor = matchAt + len(oldSeq)
	}

	result = append(result, originalLines[cursor:]...)
	return joinLines(result, trailingNewline, lineEnding), nil
}

func parsePatchHunks(chunkLines []string) ([]patchHunk, error) {
	if len(chunkLines) == 0 {
		return nil, nil
	}

	hunks := make([]patchHunk, 0, 4)
	current := patchHunk{Lines: make([]patchLine, 0, 16)}
	started := false

	flushCurrent := func() {
		if len(current.Lines) == 0 {
			return
		}
		hunks = append(hunks, current)
		current = patchHunk{Lines: make([]patchLine, 0, 16)}
	}

	for _, line := range chunkLines {
		if strings.HasPrefix(line, "@@") {
			if started {
				flushCurrent()
			}
			oldRange, newRange, hasHeader, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current.Header = line
			current.HasHeader = hasHeader
			current.OldRange = oldRange
			current.NewRange = newRange
			started = true
			continue
		}
		if line == `\ No newline at end of file` {
			continue
		}
		if line == "" {
			return nil, errors.New("patch line cannot be empty; use prefix characters")
		}
		prefix := line[0]
		if prefix != ' ' && prefix != '+' && prefix != '-' {
			return nil, fmt.Errorf("invalid patch line %q", line)
		}
		started = true
		current.Lines = append(current.Lines, patchLine{Prefix: prefix, Text: line[1:]})
	}

	flushCurrent()
	return hunks, nil
}

func parseHunkHeader(line string) (patchRange, patchRange, bool, error) {
	line = strings.TrimSpace(line)
	if line == "@@" {
		return patchRange{}, patchRange{}, false, nil
	}
	matches := unifiedHunkHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return patchRange{}, patchRange{}, false, fmt.Errorf("invalid hunk header %q", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return patchRange{}, patchRange{}, false, fmt.Errorf("invalid old start in hunk header %q", line)
	}
	oldCount, err := parseOptionalCount(matches[2])
	if err != nil {
		return patchRange{}, patchRange{}, false, fmt.Errorf("invalid old count in hunk header %q", line)
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return patchRange{}, patchRange{}, false, fmt.Errorf("invalid new start in hunk header %q", line)
	}
	newCount, err := parseOptionalCount(matches[4])
	if err != nil {
		return patchRange{}, patchRange{}, false, fmt.Errorf("invalid new count in hunk header %q", line)
	}

	return patchRange{Start: oldStart, Count: oldCount}, patchRange{Start: newStart, Count: newCount}, true, nil
}

func parseOptionalCount(raw string) (int, error) {
	if raw == "" {
		return 1, nil
	}
	return strconv.Atoi(raw)
}

func hunkSequences(hunk patchHunk) ([]string, []string) {
	oldSeq := make([]string, 0, len(hunk.Lines))
	newSeq := make([]string, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		switch line.Prefix {
		case ' ':
			oldSeq = append(oldSeq, line.Text)
			newSeq = append(newSeq, line.Text)
		case '-':
			oldSeq = append(oldSeq, line.Text)
		case '+':
			newSeq = append(newSeq, line.Text)
		}
	}
	return oldSeq, newSeq
}

func validateHunk(hunk patchHunk, oldSeq, newSeq []string) error {
	if !hunk.HasHeader {
		return nil
	}
	if hunk.OldRange.Start < 0 || hunk.NewRange.Start < 0 {
		return fmt.Errorf("invalid negative line number in hunk header %q", hunk.Header)
	}
	if hunk.OldRange.Start == 0 && hunk.OldRange.Count > 0 {
		return fmt.Errorf("invalid old range in hunk header %q", hunk.Header)
	}
	if hunk.NewRange.Start == 0 && hunk.NewRange.Count > 0 {
		return fmt.Errorf("invalid new range in hunk header %q", hunk.Header)
	}
	if len(oldSeq) != hunk.OldRange.Count {
		return fmt.Errorf("hunk header old count mismatch: expected %d lines, got %d", hunk.OldRange.Count, len(oldSeq))
	}
	if len(newSeq) != hunk.NewRange.Count {
		return fmt.Errorf("hunk header new count mismatch: expected %d lines, got %d", hunk.NewRange.Count, len(newSeq))
	}
	return nil
}

func locateHunk(lines, seq []string, start int, hunk patchHunk) (int, error) {
	if hunk.HasHeader {
		expected := hunk.OldRange.Start - 1
		if expected < 0 {
			expected = 0
		}
		if expected < start {
			return -1, fmt.Errorf("patch hunk header expected old line %d before the current patch cursor", hunk.OldRange.Start)
		}
		if expected+len(seq) > len(lines) {
			return -1, fmt.Errorf("patch hunk header expected old line %d beyond file end", hunk.OldRange.Start)
		}
		if sequenceMatchesAt(lines, seq, expected) {
			return expected, nil
		}

		matches := findAllSequenceMatches(lines, seq, start)
		if len(matches) == 0 {
			return -1, fmt.Errorf("patch hunk header expected old line %d but content did not match there", hunk.OldRange.Start)
		}
		return -1, fmt.Errorf("patch hunk header expected old line %d, but content matched line(s) %s", hunk.OldRange.Start, formatMatchLines(matches))
	}

	matches := findAllSequenceMatches(lines, seq, start)
	if len(matches) == 0 {
		return -1, fmt.Errorf("could not apply patch hunk near %q", previewLines(seq))
	}
	if len(matches) > 1 {
		return -1, fmt.Errorf("ambiguous patch hunk near %q: matched %d locations", previewLines(seq), len(matches))
	}
	return matches[0], nil
}

func sequenceMatchesAt(lines, seq []string, start int) bool {
	if start < 0 || start+len(seq) > len(lines) {
		return false
	}
	for i := range seq {
		if lines[start+i] != seq[i] {
			return false
		}
	}
	return true
}

func findAllSequenceMatches(lines, seq []string, start int) []int {
	if len(seq) == 0 || len(seq) > len(lines) {
		return nil
	}
	matches := make([]int, 0, 4)
	for i := start; i <= len(lines)-len(seq); i++ {
		if sequenceMatchesAt(lines, seq, i) {
			matches = append(matches, i)
		}
	}
	return matches
}

func formatMatchLines(matches []int) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, strconv.Itoa(match+1))
	}
	return strings.Join(parts, ", ")
}

func splitLines(text string) ([]string, bool, string) {
	lineEnding := detectLineEnding(text)
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	trailingNewline := strings.HasSuffix(normalized, "\n")
	trimmed := strings.TrimSuffix(normalized, "\n")
	if trimmed == "" {
		return []string{}, trailingNewline, lineEnding
	}
	return strings.Split(trimmed, "\n"), trailingNewline, lineEnding
}

func joinLines(lines []string, trailingNewline bool, lineEnding string) string {
	if lineEnding == "" {
		lineEnding = "\n"
	}
	joined := strings.Join(lines, lineEnding)
	if trailingNewline {
		return joined + lineEnding
	}
	return joined
}

func previewLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	joined := strings.Join(lines, " | ")
	if len(joined) > 80 {
		return joined[:80]
	}
	return joined
}

func normalizePatchText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func detectLineEnding(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
