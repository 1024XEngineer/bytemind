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

func (readFileRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		path := compactDisplayPath(result.Path)
		name := filepath.Base(result.Path)
		compactLine := name
		if result.StartLine > 0 || result.EndLine > 0 {
			compactLine = fmt.Sprintf("%s (%d-%d)", name, result.StartLine, result.EndLine)
		}
		return ToolRenderResult{
			Summary: "Read " + name,
			DetailLines: []string{
				fmt.Sprintf("range: %d-%d", result.StartLine, result.EndLine),
				"path: " + path,
			},
			Status:      "done",
			CompactLine: compactLine,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// listFilesRenderer handles "list_files" tool.
type listFilesRenderer struct{}

func (listFilesRenderer) DisplayLabel() string { return "LIST" }

func (listFilesRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Items []struct {
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
		return ToolRenderResult{
			Summary:     fmt.Sprintf("Read %d files, listed %d directories", files, dirs),
			DetailLines: []string{},
			Status:      "done",
			CompactLine: fmt.Sprintf("%d files, %d dirs", files, dirs),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// searchTextRenderer handles "search_text" tool.
type searchTextRenderer struct{}

func (searchTextRenderer) DisplayLabel() string { return "SEARCH" }

func (searchTextRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Query   string `json:"query"`
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		combined := fmt.Sprintf("%d matches for %q", len(result.Matches), result.Query)
		return ToolRenderResult{
			Summary:     combined,
			DetailLines: []string{},
			Status:      "done",
			CompactLine: combined,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// runShellRenderer handles "run_shell" tool.
type runShellRenderer struct{}

func (runShellRenderer) DisplayLabel() string { return "SHELL" }

func (runShellRenderer) Render(payload string) ToolRenderResult {
	// Check for error envelope first.
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		return ToolRenderResult{
			Summary:     compactToolText(envelope.Error, 88),
			DetailLines: nil,
			Status:      "error",
			CompactLine: compact(payload, 80),
		}
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
		return ToolRenderResult{
			Summary:     fmt.Sprintf("Shell exited with code %d", result.ExitCode),
			DetailLines: lines,
			Status:      status,
			CompactLine: fmt.Sprintf("exit code %d", result.ExitCode),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// writeFileRenderer handles "write_file" tool.
type writeFileRenderer struct{}

func (writeFileRenderer) DisplayLabel() string { return "WRITE" }

func (writeFileRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Path         string           `json:"path"`
		BytesWritten int              `json:"bytes_written"`
		DiffPreview  diffPreviewLocal `json:"diff_preview"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		name := filepath.Base(result.Path)
		if len(result.DiffPreview.Files) > 0 {
			f := result.DiffPreview.Files[0]
			compactLine := fmt.Sprintf("%s +%d -%d", name, f.Added, f.Removed)
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Created %s  +%d -%d", name, f.Added, f.Removed),
				DetailLines: diffExpandedDetailLines(result.DiffPreview),
				Status:      "done",
				CompactLine: compactLine,
			}
		}
		return ToolRenderResult{
			Summary: "Created " + name,
			DetailLines: []string{
				fmt.Sprintf("wrote %d bytes", result.BytesWritten),
			},
			Status:      "done",
			CompactLine: name,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// replaceInFileRenderer handles "replace_in_file" tool.
type replaceInFileRenderer struct{}

func (replaceInFileRenderer) DisplayLabel() string { return "EDIT" }

func (replaceInFileRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Path        string           `json:"path"`
		Replaced    int              `json:"replaced"`
		OldCount    int              `json:"old_count"`
		DiffPreview diffPreviewLocal `json:"diff_preview"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		name := filepath.Base(result.Path)
		if len(result.DiffPreview.Files) > 0 {
			f := result.DiffPreview.Files[0]
			compactLine := fmt.Sprintf("%s +%d -%d", name, f.Added, f.Removed)
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Updated %s  +%d -%d", name, f.Added, f.Removed),
				DetailLines: diffExpandedDetailLines(result.DiffPreview),
				Status:      "done",
				CompactLine: compactLine,
			}
		}
		return ToolRenderResult{
			Summary: "Updated " + name,
			DetailLines: []string{
				fmt.Sprintf("replaced %d lines", result.Replaced),
			},
			Status:      "done",
			CompactLine: fmt.Sprintf("%s (%d lines)", name, result.Replaced),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// applyPatchRenderer handles "apply_patch" tool.
type applyPatchRenderer struct{}

func (applyPatchRenderer) DisplayLabel() string { return "PATCH" }

func (applyPatchRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Operations  []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"operations"`
		DiffPreview diffPreviewLocal `json:"diff_preview"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		if len(result.DiffPreview.Files) > 0 {
			compactLine := fmt.Sprintf("%d files, +%d -%d", result.DiffPreview.TotalFiles, result.DiffPreview.TotalAdded, result.DiffPreview.TotalRemoved)
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Updated %d files  +%d -%d", result.DiffPreview.TotalFiles, result.DiffPreview.TotalAdded, result.DiffPreview.TotalRemoved),
				DetailLines: diffExpandedDetailLines(result.DiffPreview),
				Status:      "done",
				CompactLine: compactLine,
			}
		}
		operationLines := make([]string, 0, min(10, len(result.Operations)))
		for i := 0; i < min(10, len(result.Operations)); i++ {
			operationLines = append(operationLines, result.Operations[i].Type+" "+compactDisplayPath(result.Operations[i].Path))
		}
		if len(result.Operations) > 10 {
			operationLines = append(operationLines, "...")
		}
		compactLine := fmt.Sprintf("%d files", len(result.Operations))
		if len(result.Operations) == 1 {
			compactLine = filepath.Base(result.Operations[0].Path)
		}
		return ToolRenderResult{
			Summary:     fmt.Sprintf("Updated %d files", len(result.Operations)),
			DetailLines: operationLines,
			Status:      "done",
			CompactLine: compactLine,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// updatePlanRenderer handles "update_plan" tool.
type updatePlanRenderer struct{}

func (updatePlanRenderer) DisplayLabel() string { return "PLAN" }

func (updatePlanRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		Plan planpkg.State `json:"plan"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		lines := make([]string, 0, min(4, len(result.Plan.Steps)))
		for i := 0; i < min(4, len(result.Plan.Steps)); i++ {
			step := result.Plan.Steps[i]
			lines = append(lines, fmt.Sprintf("[%s] %s", step.Status, step.Title))
		}
		return ToolRenderResult{
			Summary:     fmt.Sprintf("Updated plan with %d step(s)", len(result.Plan.Steps)),
			DetailLines: lines,
			Status:      "done",
			CompactLine: fmt.Sprintf("%d step(s)", len(result.Plan.Steps)),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// webSearchRenderer handles "web_search" tool.
type webSearchRenderer struct{}

func (webSearchRenderer) DisplayLabel() string { return "SEARCH" }

func (webSearchRenderer) Render(payload string) ToolRenderResult {
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
		return ToolRenderResult{
			Summary:     fmt.Sprintf("Web search for %q", result.Query),
			DetailLines: lines,
			Status:      "done",
			CompactLine: fmt.Sprintf("%d results for %q", len(result.Results), result.Query),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// webFetchRenderer handles "web_fetch" tool.
type webFetchRenderer struct{}

func (webFetchRenderer) DisplayLabel() string { return "FETCH" }

func (webFetchRenderer) Render(payload string) ToolRenderResult {
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
		return ToolRenderResult{
			Summary:     "Fetched " + compact(result.URL, 56),
			DetailLines: lines,
			Status:      "done",
			CompactLine: compact(result.URL, 60),
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}
