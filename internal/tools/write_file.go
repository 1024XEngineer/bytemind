package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/1024XEngineer/bytemind/internal/llm"
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

func (WriteFileTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
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
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", err
	}

	return toJSON(map[string]any{
		"ok":            true,
		"path":          filepath.ToSlash(mustRel(execCtx.Workspace, path)),
		"bytes_written": len(args.Content),
	})
}
