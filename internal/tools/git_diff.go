package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type GitDiffTool struct{}

func (GitDiffTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "git_diff",
			Description: "Show the diff of staged or unstaged changes in the git repository. Returns a unified diff output.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"staged": map[string]any{
						"type":        "boolean",
						"description": "Show staged changes (--cached). Defaults to false (unstaged changes).",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional subdirectory or file path relative to workspace. Defaults to workspace root.",
					},
				},
			},
		},
	}
}

func (GitDiffTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	dir := execCtx.Workspace
	var args struct {
		Staged bool   `json:"staged"`
		Path   string `json:"path"`
	}
	if json.Unmarshal(raw, &args) == nil && strings.TrimSpace(args.Path) != "" {
		resolved, err := resolvePath(execCtx.Workspace, args.Path)
		if err == nil {
			dir = resolved
		}
	}

	diffArgs := []string{"-C", dir, "diff", "--unified=8"}
	if args.Staged {
		diffArgs = append(diffArgs, "--cached")
	}
	cmd := exec.CommandContext(ctx, "git", diffArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return toJSON(map[string]any{
			"ok":      false,
			"error":   "git diff failed",
			"details": strings.TrimSpace(stderr.String()),
		})
	}

	diff := stdout.String()
	diffLines := strings.Split(diff, "\n")
	if len(diffLines) > 0 && diffLines[len(diffLines)-1] == "" {
		diffLines = diffLines[:len(diffLines)-1]
	}

	added := 0
	removed := 0
	for _, line := range diffLines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}

	files := make([]string, 0)
	for _, line := range diffLines {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				name := strings.TrimPrefix(parts[3], "b/")
				files = append(files, name)
			}
		}
	}

	truncated := false
	display := diff
	if len(diff) > 32*1024 {
		display = diff[:32*1024] + "\n... (diff truncated at 32KB)"
		truncated = true
	}

	summary := fmt.Sprintf("%d file(s), +%d/-%d lines", len(files), added, removed)
	if len(files) == 0 {
		summary = "no changes"
	}

	return toJSON(map[string]any{
		"ok":        true,
		"diff":      display,
		"files":     files,
		"added":     added,
		"removed":   removed,
		"summary":   summary,
		"truncated": truncated,
	})
}
