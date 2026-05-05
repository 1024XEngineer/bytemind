package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

// readFileRenderer handles "read_file" tool.
type readFileRenderer struct{}

func (readFileRenderer) DisplayLabel() string { return "READ" }

func (readFileRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		path := compactDisplayPath(result.Path)
		summary := "Read " + filepath.Base(result.Path)
		return summary, []string{
			fmt.Sprintf("range: %d-%d", result.StartLine, result.EndLine),
			"path: " + path,
		}, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (readFileRenderer) CompactLine(payload string) string {
	var result struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		name := filepath.Base(result.Path)
		if result.StartLine > 0 || result.EndLine > 0 {
			return fmt.Sprintf("%s (%d-%d)", name, result.StartLine, result.EndLine)
		}
		return name
	}
	return compact(payload, 80)
}

// listFilesRenderer handles "list_files" tool.
type listFilesRenderer struct{}

func (listFilesRenderer) DisplayLabel() string { return "LIST" }

func (listFilesRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Root  string `json:"root"`
		Items []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		dirs := 0
		files := 0
		for _, item := range result.Items {
			if item.Type == "dir" {
				dirs++
			} else {
				files++
			}
		}
		return fmt.Sprintf("Read %d files, listed %d directories", files, dirs), []string{}, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (listFilesRenderer) CompactLine(payload string) string {
	var result struct {
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		files := 0
		dirs := 0
		for _, item := range result.Items {
			if item.Type == "dir" {
				dirs++
			} else {
				files++
			}
		}
		return fmt.Sprintf("%d files, %d dirs", files, dirs)
	}
	return compact(payload, 80)
}

// searchTextRenderer handles "search_text" tool.
type searchTextRenderer struct{}

func (searchTextRenderer) DisplayLabel() string { return "SEARCH" }

func (searchTextRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Query   string `json:"query"`
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("%d matches for %q", len(result.Matches), result.Query), []string{}, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (searchTextRenderer) CompactLine(payload string) string {
	var result struct {
		Query   string `json:"query"`
		Matches []struct{} `json:"matches"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("%d matches for %q", len(result.Matches), result.Query)
	}
	return compact(payload, 80)
}

// runShellRenderer handles "run_shell" tool.
type runShellRenderer struct{}

func (runShellRenderer) DisplayLabel() string { return "SHELL" }

func (runShellRenderer) ResultSummary(payload string) (string, []string, string) {
	// Check for error envelope first
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		return compactToolText(envelope.Error, 88), nil, "error"
	}

	var result struct {
		OK       bool   `json:"ok"`
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		lines := make([]string, 0, 2)
		if text := strings.TrimSpace(result.Stdout); text != "" {
			lines = append(lines, "stdout: "+compact(strings.Split(text, "\n")[0], 64))
		}
		if text := strings.TrimSpace(result.Stderr); text != "" {
			lines = append(lines, "stderr: "+compact(strings.Split(text, "\n")[0], 64))
		}
		status := "done"
		if !result.OK {
			status = "warn"
		}
		return fmt.Sprintf("Shell exited with code %d", result.ExitCode), lines, status
	}
	return compact(payload, 96), nil, "done"
}

func (runShellRenderer) CompactLine(payload string) string {
	var result struct {
		ExitCode int `json:"exit_code"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("exit code %d", result.ExitCode)
	}
	return compact(payload, 80)
}

// writeFileRenderer handles "write_file" tool.
type writeFileRenderer struct{}

func (writeFileRenderer) DisplayLabel() string { return "WRITE" }

func (writeFileRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return "创建 " + filepath.Base(result.Path), []string{
			fmt.Sprintf("写入 %d 字节", result.BytesWritten),
		}, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (writeFileRenderer) CompactLine(payload string) string {
	var result struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return filepath.Base(result.Path)
	}
	return compact(payload, 80)
}

// replaceInFileRenderer handles "replace_in_file" tool.
type replaceInFileRenderer struct{}

func (replaceInFileRenderer) DisplayLabel() string { return "EDIT" }

func (replaceInFileRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Path     string `json:"path"`
		Replaced int    `json:"replaced"`
		OldCount int    `json:"old_count"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return "改动 " + filepath.Base(result.Path), []string{
			fmt.Sprintf("改动 %d 行", result.Replaced),
		}, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (replaceInFileRenderer) CompactLine(payload string) string {
	var result struct {
		Path     string `json:"path"`
		Replaced int    `json:"replaced"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("%s (%d lines)", filepath.Base(result.Path), result.Replaced)
	}
	return compact(payload, 80)
}

// applyPatchRenderer handles "apply_patch" tool.
type applyPatchRenderer struct{}

func (applyPatchRenderer) DisplayLabel() string { return "PATCH" }

func (applyPatchRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Operations []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"operations"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		operationLines := make([]string, 0, min(10, len(result.Operations)))
		for i := 0; i < min(10, len(result.Operations)); i++ {
			operationLines = append(operationLines, result.Operations[i].Type+" "+compactDisplayPath(result.Operations[i].Path))
		}
		if len(result.Operations) > 10 {
			operationLines = append(operationLines, "...")
		}
		return fmt.Sprintf("改动 %d 个文件", len(result.Operations)), operationLines, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (applyPatchRenderer) CompactLine(payload string) string {
	var result struct {
		Operations []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"operations"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		if len(result.Operations) == 1 {
			return filepath.Base(result.Operations[0].Path)
		}
		return fmt.Sprintf("%d files", len(result.Operations))
	}
	return compact(payload, 80)
}

// updatePlanRenderer handles "update_plan" tool.
type updatePlanRenderer struct{}

func (updatePlanRenderer) DisplayLabel() string { return "PLAN" }

func (updatePlanRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Plan planpkg.State `json:"plan"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		lines := make([]string, 0, min(4, len(result.Plan.Steps)))
		for i := 0; i < min(4, len(result.Plan.Steps)); i++ {
			step := result.Plan.Steps[i]
			lines = append(lines, fmt.Sprintf("[%s] %s", step.Status, step.Title))
		}
		return fmt.Sprintf("Updated plan with %d step(s)", len(result.Plan.Steps)), lines, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (updatePlanRenderer) CompactLine(payload string) string {
	var result struct {
		Plan planpkg.State `json:"plan"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("%d step(s)", len(result.Plan.Steps))
	}
	return compact(payload, 80)
}

// webSearchRenderer handles "web_search" tool.
type webSearchRenderer struct{}

func (webSearchRenderer) DisplayLabel() string { return "SEARCH" }

func (webSearchRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		Query   string `json:"query"`
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		lines := []string{fmt.Sprintf("results: %d", len(result.Results))}
		for i := 0; i < min(3, len(result.Results)); i++ {
			item := result.Results[i]
			title := compact(item.Title, 52)
			if strings.TrimSpace(title) == "" {
				title = compact(item.URL, 52)
			}
			lines = append(lines, title+" - "+compact(item.URL, 52))
		}
		return fmt.Sprintf("Web search for %q", result.Query), lines, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (webSearchRenderer) CompactLine(payload string) string {
	var result struct {
		Query   string   `json:"query"`
		Results []struct{} `json:"results"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return fmt.Sprintf("%d results for %q", len(result.Results), result.Query)
	}
	return compact(payload, 80)
}

// webFetchRenderer handles "web_fetch" tool.
type webFetchRenderer struct{}

func (webFetchRenderer) DisplayLabel() string { return "FETCH" }

func (webFetchRenderer) ResultSummary(payload string) (string, []string, string) {
	var result struct {
		URL        string `json:"url"`
		StatusCode int    `json:"status_code"`
		Title      string `json:"title"`
		Content    string `json:"content"`
		Truncated  bool   `json:"truncated"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		lines := []string{fmt.Sprintf("status: HTTP %d", result.StatusCode)}
		if strings.TrimSpace(result.Title) != "" {
			lines = append(lines, "title: "+compact(result.Title, 64))
		}
		if strings.TrimSpace(result.Content) != "" {
			lines = append(lines, "preview: "+compactToolText(result.Content, 64))
		}
		if result.Truncated {
			lines = append(lines, "content: truncated")
		}
		return "Fetched " + compact(result.URL, 56), lines, "done"
	}
	return compact(payload, 96), nil, "done"
}

func (webFetchRenderer) CompactLine(payload string) string {
	var result struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		return compact(result.URL, 60)
	}
	return compact(payload, 80)
}
