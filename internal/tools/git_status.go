package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type GitStatusTool struct{}

func (GitStatusTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "git_status",
			Description: "Show the working tree status of the git repository. Returns staged, unstaged, and untracked file lists.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Optional subdirectory path relative to workspace. Defaults to workspace root.",
					},
				},
			},
		},
	}
}

func (GitStatusTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	dir := execCtx.Workspace
	var args struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(raw, &args) == nil && strings.TrimSpace(args.Path) != "" {
		resolved, err := resolvePath(execCtx.Workspace, args.Path)
		if err == nil {
			dir = resolved
		}
	}

	statusOut, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return toJSON(map[string]any{
			"ok":      false,
			"error":   "not a git repository or git unavailable",
			"details": err.Error(),
		})
	}

	branchOut, _ := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" {
		hashOut, _ := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
		branch = "detached at " + strings.TrimSpace(string(hashOut))
	}

	rawLines := strings.Split(strings.TrimSpace(string(statusOut)), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}

	staged := make([]string, 0)
	unstaged := make([]string, 0)
	untracked := make([]string, 0)
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		file := strings.TrimSpace(line[2:])
		switch {
		case status == "??":
			untracked = append(untracked, file)
		case status[0] != ' ' && status[0] != '?':
			staged = append(staged, file)
		case status[1] != ' ':
			unstaged = append(unstaged, file)
		}
	}

	total := len(staged) + len(unstaged) + len(untracked)
	parts := make([]string, 0, 3)
	if len(staged) > 0 {
		parts = append(parts, strconv.Itoa(len(staged))+" staged")
	}
	if len(unstaged) > 0 {
		parts = append(parts, strconv.Itoa(len(unstaged))+" unstaged")
	}
	if len(untracked) > 0 {
		parts = append(parts, strconv.Itoa(len(untracked))+" untracked")
	}
	summary := fmt.Sprintf("clean")
	if len(parts) > 0 {
		summary = strings.Join(parts, ", ") + " on " + branch
	}

	return toJSON(map[string]any{
		"ok":        true,
		"branch":    branch,
		"staged":    staged,
		"unstaged":  unstaged,
		"untracked": untracked,
		"total":     total,
		"summary":   summary,
	})
}
