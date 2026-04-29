package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type ReadFileTool struct{}

func (ReadFileTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "read_file",
			Description: "Read a text file from the workspace or configured writable roots",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path from workspace or absolute path inside workspace/writable_roots.",
					},
					"start_line": map[string]any{
						"type":        "integer",
						"description": "Optional 1-based start line.",
					},
					"end_line": map[string]any{
						"type":        "integer",
						"description": "Optional 1-based end line, inclusive.",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (ReadFileTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
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
	if !isText(data) {
		return "", errors.New("file is not a text file")
	}

	lines := make([]string, 0, 128)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	start := 1
	if args.StartLine > 0 {
		start = args.StartLine
	}
	end := len(lines)
	if args.EndLine > 0 && args.EndLine < end {
		end = args.EndLine
	}
	if len(lines) == 0 {
		start, end = 0, 0
	}
	if len(lines) > 0 && start > len(lines) {
		start = len(lines) + 1
		end = len(lines)
	}
	if start < 1 && len(lines) > 0 {
		start = 1
	}
	if end < 0 {
		end = 0
	}

	content := ""
	if len(lines) > 0 && start > 0 && start <= len(lines) && end >= start {
		content = strings.Join(lines[start-1:end], "\n")
	}

	return toJSON(map[string]any{
		"ok":          true,
		"path":        filepath.ToSlash(mustRel(execCtx.Workspace, path)),
		"start_line":  start,
		"end_line":    end,
		"total_lines": len(lines),
		"content":     content,
	})
}
