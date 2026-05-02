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

	return toJSON(map[string]any{
		"ok":        true,
		"path":      filepath.ToSlash(mustRel(execCtx.Workspace, path)),
		"replaced":  replaced,
		"old_count": count,
	})
}
