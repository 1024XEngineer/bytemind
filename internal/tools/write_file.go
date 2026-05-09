package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
)

type WriteFileTool struct{}

func (WriteFileTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "write_file",
			Description: "Write or create a file inside the workspace or configured writable roots",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path from workspace or absolute path inside workspace/writable_roots.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full file content to write.",
					},
					"create_dirs": map[string]any{
						"type":        "boolean",
						"description": "Create parent directories when needed.",
					},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func (WriteFileTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		CreateDirs bool   `json:"create_dirs"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	path, err := resolvePath(execCtx.Workspace, args.Path, writableRootsFromExecContext(execCtx)...)
	if err != nil {
		return "", err
	}
	if args.CreateDirs {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
	}

	relPath := filepath.ToSlash(mustRel(execCtx.Workspace, path))
	_, statErr := os.Stat(path)
	exists := statErr == nil

	var original string
	if exists {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		original = string(data)
	}

	opType := rollbackpkg.OpTypeAdd
	if exists {
		opType = rollbackpkg.OpTypeUpdate
	}
	tracker, err := beginRollbackOperation(ctx, execCtx, "write_file", []rollbackpkg.FileTarget{{
		Path:    relPath,
		AbsPath: path,
		OpType:  opType,
	}})
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		if tracker != nil {
			tracker.abort(ctx, err.Error())
		}
		return "", err
	}
	operationID, err := tracker.commit(ctx)
	if err != nil {
		return "", err
	}

	result := map[string]any{
		"ok":            true,
		"path":          relPath,
		"bytes_written": len(args.Content),
	}
	if operationID != "" {
		result["rollback_operation_id"] = operationID
	}

	if dp := buildWriteFileDiff(original, args.Content, exists, relPath); dp != nil {
		result["diff_preview"] = dp
	}

	return toJSON(result)
}

func buildWriteFileDiff(original, content string, existed bool, relPath string) *DiffPreview {
	if !existed {
		return newFileDiff(content, relPath)
	}
	return overwriteFileDiff(original, content, relPath)
}

func newFileDiff(content, relPath string) *DiffPreview {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil
	}
	added := len(lines)
	hunks := contentToAddHunk(lines)
	dp := &DiffPreview{
		Files: []DiffFile{{
			Path:       relPath,
			ChangeType: "add",
			Added:      added,
			Hunks:      hunks,
		}},
		TotalFiles:   1,
		TotalAdded:   added,
		TotalRemoved: 0,
	}
	sanitizeDiffPreview(dp)
	return dp
}

func overwriteFileDiff(original, content, relPath string) *DiffPreview {
	origLines := strings.Split(original, "\n")
	newLines := strings.Split(content, "\n")

	diffAdded := len(newLines) - len(origLines)
	diffRemoved := 0
	if diffAdded < 0 {
		diffRemoved = -diffAdded
		diffAdded = 0
	}

	hunkLines := make([]string, 0, 10)
	maxLen := len(origLines)
	if len(newLines) < maxLen {
		maxLen = len(newLines)
	}
	show := maxLen
	if show > 6 {
		show = 6
	}
	for i := 0; i < show; i++ {
		if i < len(origLines) && i < len(newLines) && origLines[i] != newLines[i] {
			hunkLines = append(hunkLines, "-"+origLines[i])
			hunkLines = append(hunkLines, "+"+newLines[i])
		}
	}
	if len(hunkLines) == 0 && len(origLines) != len(newLines) {
		if len(origLines) > 0 {
			hunkLines = append(hunkLines, "-"+origLines[0])
		}
		if len(newLines) > 0 {
			hunkLines = append(hunkLines, "+"+newLines[0])
		}
	}

	truncated := len(hunkLines) > diffMaxLinesPerHunk
	if truncated {
		hunkLines = hunkLines[:diffMaxLinesPerHunk]
	}

	dp := &DiffPreview{
		Files: []DiffFile{{
			Path:       relPath,
			ChangeType: "modify",
			Added:      diffAdded,
			Removed:    diffRemoved,
			Hunks:      []DiffHunk{{OldStart: 1, OldLines: len(origLines), NewStart: 1, NewLines: len(newLines), Lines: hunkLines}},
		}},
		TotalFiles:   1,
		TotalAdded:   diffAdded,
		TotalRemoved: diffRemoved,
		Truncated:    truncated,
	}
	sanitizeDiffPreview(dp)
	return dp
}
