package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bytemind/internal/llm"
)

type SearchTextTool struct{}
type searchTextArgs struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	Limit         int    `json:"limit"`
	CaseSensitive bool   `json:"case_sensitive"`
}

type searchTextMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type searchTextScanStats struct {
	Visits            int
	FilesVisited      int
	FilesScanned      int
	BytesScanned      int64
	SkippedLargeFiles int
	Truncated         bool
	Reason            string
}

var searchTextLookPath = exec.LookPath
var searchTextCommand = exec.CommandContext

func (SearchTextTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "search_text",
			Description: "Search text across workspace files",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Text to search for. Use a focused query.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional directory or file to limit the search. Use this on large workspaces.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of matches to return. Defaults to 50.",
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "Whether the search is case sensitive.",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (SearchTextTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args searchTextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", errors.New("query is required")
	}
	if args.Limit <= 0 {
		args.Limit = 50
	}
	if args.Limit > 200 {
		args.Limit = 200
	}
	root, err := resolvePath(execCtx.Workspace, args.Path)
	if err != nil {
		return "", err
	}
	if matches, truncated, reason, used, rgErr := searchTextWithRipgrep(ctx, args, execCtx.Workspace, root); rgErr != nil {
		return "", rgErr
	} else if used {
		result := map[string]any{
			"ok":      true,
			"query":   args.Query,
			"matches": matches,
			"engine":  "ripgrep",
		}
		if truncated {
			result["truncated"] = true
			result["reason"] = reason
		}
		return toJSON(result)
	}

	matches, stats, err := searchTextByWalking(ctx, args, execCtx.Workspace, root)
	if err != nil {
		return "", err
	}

	result := map[string]any{
		"ok":      true,
		"query":   args.Query,
		"matches": matches,
		"engine":  "walkdir",
	}
	if stats.Truncated {
		result["truncated"] = true
		result["reason"] = stats.Reason
		result["max_visits"] = maxSearchTextVisits()
		result["max_files"] = maxSearchTextFiles()
		result["max_bytes"] = maxSearchTextBytes()
		result["visits"] = stats.Visits
		result["files_visited"] = stats.FilesVisited
		result["files_scanned"] = stats.FilesScanned
		result["bytes_scanned"] = stats.BytesScanned
		result["skipped_large_files"] = stats.SkippedLargeFiles
	}
	return toJSON(result)
}

func searchTextWithRipgrep(ctx context.Context, args searchTextArgs, workspace, root string) ([]searchTextMatch, bool, string, bool, error) {
	if !searchTextCanUseRipgrepBudgets() {
		return nil, false, "", false, nil
	}
	if _, err := searchTextLookPath("rg"); err != nil {
		return nil, false, "", false, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, false, "", true, ctxErr
	}

	searchBase := root
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		searchBase = filepath.Dir(root)
	}

	rgCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	commandArgs := []string{
		"--json",
		"--line-number",
		"--no-heading",
		"--color", "never",
		"--fixed-strings",
		"--max-filesize", fmt.Sprintf("%d", maxSearchTextFileBytes()),
	}
	if args.CaseSensitive {
		commandArgs = append(commandArgs, "--case-sensitive")
	} else {
		commandArgs = append(commandArgs, "--ignore-case")
	}
	for _, glob := range []string{
		"**/node_modules/**",
		"**/vendor/**",
		"**/dist/**",
		"**/build/**",
		"**/target/**",
		"**/coverage/**",
		"**/.next/**",
		"**/.nuxt/**",
		"**/out/**",
		"**/bin/**",
		"**/obj/**",
	} {
		commandArgs = append(commandArgs, "--glob", "!"+glob)
	}
	commandArgs = append(commandArgs, "--", args.Query, root)

	cmd := searchTextCommand(rgCtx, "rg", commandArgs...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, "", false, nil
	}
	if err := cmd.Start(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, "", true, ctxErr
		}
		return nil, false, "", false, nil
	}

	matches := make([]searchTextMatch, 0, args.Limit)
	truncated := false
	reason := ""
	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				LineNumber int `json:"line_number"`
				Lines      struct {
					Text string `json:"text"`
				} `json:"lines"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Type != "match" {
			continue
		}
		matchPath := normalizeRipgrepPath(workspace, searchBase, event.Data.Path.Text)
		if matchPath == "" {
			continue
		}
		matches = append(matches, searchTextMatch{
			Path: matchPath,
			Line: event.Data.LineNumber,
			Text: strings.TrimRight(event.Data.Lines.Text, "\r\n"),
		})
		if len(matches) >= args.Limit {
			truncated = true
			reason = "result_limit"
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, false, "", true, ctxErr
	}
	if scanErr != nil {
		return nil, false, "", false, nil
	}
	if truncated {
		return matches, true, reason, true, nil
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 && len(matches) == 0 {
			return matches, false, "", true, nil
		}
		return nil, false, "", false, nil
	}
	return matches, false, "", true, nil
}

func searchTextByWalking(ctx context.Context, args searchTextArgs, workspace, root string) ([]searchTextMatch, searchTextScanStats, error) {
	maxVisits := maxSearchTextVisits()
	maxFiles := maxSearchTextFiles()
	maxBytes := maxSearchTextBytes()
	maxFileBytes := maxSearchTextFileBytes()

	needle := args.Query
	if !args.CaseSensitive {
		needle = strings.ToLower(needle)
	}

	matches := make([]searchTextMatch, 0, args.Limit)
	stats := searchTextScanStats{}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != root {
			stats.Visits++
			if stats.Visits > maxVisits {
				stats.Truncated = true
				stats.Reason = "visit_limit"
				return fs.SkipAll
			}
		}
		if d.IsDir() {
			if path != root && (isHidden(d.Name()) || shouldSkipToolDir(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		stats.FilesVisited++
		if stats.FilesVisited > maxFiles {
			stats.Truncated = true
			stats.Reason = "file_limit"
			return fs.SkipAll
		}
		if info, infoErr := d.Info(); infoErr == nil {
			if info.Size() > maxFileBytes {
				stats.SkippedLargeFiles++
				return nil
			}
			if stats.BytesScanned+info.Size() > maxBytes {
				stats.Truncated = true
				stats.Reason = "byte_limit"
				return fs.SkipAll
			}
		}

		data, err := os.ReadFile(path)
		if err != nil || !isText(data) {
			return nil
		}
		stats.BytesScanned += int64(len(data))
		if stats.BytesScanned > maxBytes {
			stats.Truncated = true
			stats.Reason = "byte_limit"
			return fs.SkipAll
		}
		stats.FilesScanned++

		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			text := scanner.Text()
			haystack := text
			if !args.CaseSensitive {
				haystack = strings.ToLower(text)
			}
			if strings.Contains(haystack, needle) {
				matches = append(matches, searchTextMatch{
					Path: filepath.ToSlash(mustRel(workspace, path)),
					Line: lineNumber,
					Text: text,
				})
				if len(matches) >= args.Limit {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return nil, searchTextScanStats{}, walkErr
	}
	return matches, stats, nil
}

func searchTextCanUseRipgrepBudgets() bool {
	return maxSearchTextVisits() == defaultSearchTextMaxVisits &&
		maxSearchTextFiles() == defaultSearchTextMaxFiles &&
		maxSearchTextBytes() == defaultSearchTextMaxBytes &&
		maxSearchTextFileBytes() == defaultSearchTextMaxFileBytes
}

func normalizeRipgrepPath(workspace, base, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.ToSlash(mustRel(workspace, path))
}
