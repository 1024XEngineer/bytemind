package tools

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type ListFilesTool struct{}

func (ListFilesTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "list_files",
			Description: "List files and directories inside the workspace or configured writable roots",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path from workspace or absolute path inside workspace/writable_roots. Defaults to workspace root.",
					},
					"depth": map[string]any{
						"type":        "integer",
						"description": "Maximum traversal depth. Defaults to 4.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of returned entries. Defaults to 200.",
					},
					"include_hidden": map[string]any{
						"type":        "boolean",
						"description": "Whether to include hidden files and directories.",
					},
				},
			},
		},
	}
}

func (ListFilesTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Path          string `json:"path"`
		Depth         int    `json:"depth"`
		Limit         int    `json:"limit"`
		IncludeHidden bool   `json:"include_hidden"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if args.Depth <= 0 {
		args.Depth = 4
	}
	if args.Depth > 12 {
		args.Depth = 12
	}
	if args.Limit <= 0 {
		args.Limit = 200
	}
	if args.Limit > 1000 {
		args.Limit = 1000
	}

	root, err := resolvePath(execCtx.Workspace, args.Path, writableRootsFromExecContext(execCtx)...)
	if err != nil {
		return "", err
	}
	maxVisits := maxListFilesVisits()

	type entry struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size,omitempty"`
	}

	items := make([]entry, 0, args.Limit)
	visits := 0
	truncated := false
	stopReason := ""
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		visits++
		if visits > maxVisits {
			truncated = true
			stopReason = "visit_limit"
			return fs.SkipAll
		}
		if d.IsDir() && shouldSkipToolDir(d.Name()) {
			return filepath.SkipDir
		}
		if !args.IncludeHidden && isHidden(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if depthFromRoot(root, path) > args.Depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(execCtx.Workspace, path)
		if err != nil {
			return err
		}
		item := entry{
			Path: filepath.ToSlash(rel),
			Type: "file",
		}
		if d.IsDir() {
			item.Type = "dir"
		} else if info, statErr := d.Info(); statErr == nil {
			item.Size = info.Size()
		}
		items = append(items, item)
		if len(items) >= args.Limit {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && walkErr != fs.SkipAll {
		return "", walkErr
	}

	result := map[string]any{
		"ok":    true,
		"root":  filepath.ToSlash(mustRel(execCtx.Workspace, root)),
		"items": items,
	}
	if truncated {
		result["truncated"] = true
		result["reason"] = stopReason
		result["max_visits"] = maxVisits
	}
	return toJSON(result)
}
