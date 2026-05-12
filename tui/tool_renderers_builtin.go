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
			compactLine := name
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Created %s", name),
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
			compactLine := name
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Updated %s", name),
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
			compactLine := fmt.Sprintf("%d files", result.DiffPreview.TotalFiles)
			return ToolRenderResult{
				Summary:     fmt.Sprintf("Updated %d files", result.DiffPreview.TotalFiles),
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

// delegateSubAgentRenderer handles "delegate_subagent" tool.
type delegateSubAgentRenderer struct{}

func (delegateSubAgentRenderer) DisplayLabel() string { return "SUBAGENT" }

func (delegateSubAgentRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		OK       bool   `json:"ok"`
		Status   string `json:"status,omitempty"`
		Agent    string `json:"agent"`
		Task     string `json:"task,omitempty"`
		Summary  string `json:"summary,omitempty"`
		Content  string `json:"content,omitempty"`
		Error    *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
		Transcript []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"transcript,omitempty"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		status := "done"
		if !result.OK {
			status = "error"
		} else if strings.HasPrefix(strings.TrimSpace(result.Summary), "SubAgent error:") {
			status = "warn"
		} else if result.Status == "running" || result.Status == "accepted" {
			status = "running"
		}

		// Prefer Content (natural language) over Summary (structured) for display.
		displayText := strings.TrimSpace(result.Content)
		if displayText == "" {
			displayText = strings.TrimSpace(result.Summary)
		}

		// Summary: stats overview (agent name + summary or tool count).
		summary := fmt.Sprintf("SubAgent %s", result.Agent)
		if text := displayText; text != "" {
			summary += ": " + compact(text, 72)
		}

		// DetailLines (Ctrl+O expanded): Prompt + transcript + Response.
		detailLines := make([]string, 0, len(result.Transcript)+4)
		if task := strings.TrimSpace(result.Task); task != "" {
			detailLines = append(detailLines, "Prompt: "+compactToolText(task, 72))
			detailLines = append(detailLines, "")
		}
		for _, msg := range result.Transcript {
			text := compactToolText(msg.Content, 72)
			if text != "" {
				detailLines = append(detailLines, text)
			}
		}
		if text := displayText; text != "" {
			detailLines = append(detailLines, "")
			detailLines = append(detailLines, "Response: "+compactToolText(text, 200))
		}

		compactLine := result.Agent
		if task := strings.TrimSpace(result.Task); task != "" {
			compactLine += "(" + compact(task, 48) + ")"
		} else if text := strings.TrimSpace(result.Summary); text != "" {
			compactLine += ": " + compact(text, 60)
		}

		return ToolRenderResult{
			Summary:     summary,
			DetailLines: detailLines,
			Status:      status,
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

// gitStatusRenderer handles "git_status" tool.
type gitStatusRenderer struct{}

func (gitStatusRenderer) DisplayLabel() string { return "GIT" }

func (gitStatusRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		OK        bool     `json:"ok"`
		Branch    string   `json:"branch"`
		Staged    []string `json:"staged"`
		Unstaged  []string `json:"unstaged"`
		Untracked []string `json:"untracked"`
		Summary   string   `json:"summary"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil && result.OK {
		lines := make([]string, 0, 3)
		if len(result.Staged) > 0 {
			lines = append(lines, "staged: "+strings.Join(result.Staged, ", "))
		}
		if len(result.Unstaged) > 0 {
			lines = append(lines, "unstaged: "+strings.Join(result.Unstaged, ", "))
		}
		if len(result.Untracked) > 0 {
			lines = append(lines, "untracked: "+strings.Join(result.Untracked, ", "))
		}
		return ToolRenderResult{
			Summary:     result.Summary,
			DetailLines: lines,
			Status:      "done",
			CompactLine: result.Summary,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// gitDiffRenderer handles "git_diff" tool.
type gitDiffRenderer struct{}

func (gitDiffRenderer) DisplayLabel() string { return "DIFF" }

func (gitDiffRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		OK      bool     `json:"ok"`
		Files   []string `json:"files"`
		Added   int      `json:"added"`
		Removed int      `json:"removed"`
		Summary string   `json:"summary"`
		Diff    string   `json:"diff"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil && result.OK {
		lines := make([]string, 0, len(result.Files)+2)
		for _, f := range result.Files {
			lines = append(lines, f)
		}
		if len(result.Files) > 0 {
			lines = append(lines, "")
			diffLines := strings.Split(result.Diff, "\n")
			maxPreview := 20
			for i, dl := range diffLines {
				if i >= maxPreview {
					lines = append(lines, "...")
					break
				}
				lines = append(lines, dl)
			}
		}
		return ToolRenderResult{
			Summary:     result.Summary,
			DetailLines: lines,
			Status:      "done",
			CompactLine: result.Summary,
		}
	}
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

// runTestsRenderer handles "run_tests" tool.
type runTestsRenderer struct{}

func (runTestsRenderer) DisplayLabel() string { return "TEST" }

func (runTestsRenderer) Render(payload string) ToolRenderResult {
	var result struct {
		OK       bool    `json:"ok"`
		Passed   int     `json:"passed"`
		Failed   int     `json:"failed"`
		Skipped  int     `json:"skipped"`
		ExitCode int     `json:"exit_code"`
		ElapsedS float64 `json:"elapsed_s"`
		Command  string  `json:"command"`
		Stdout   string  `json:"stdout"`
		Summary  string  `json:"summary"`
	}
	if json.Unmarshal([]byte(payload), &result) == nil {
		status := "done"
		if !result.OK {
			status = "warn"
		}
		lines := make([]string, 0, 6)
		lines = append(lines, "command: "+result.Command)
		lines = append(lines, fmt.Sprintf("tests: passed=%d failed=%d skipped=%d (%.1fs)", result.Passed, result.Failed, result.Skipped, result.ElapsedS))
		if firstLine := strings.Split(strings.TrimSpace(result.Stdout), "\n"); len(firstLine) > 0 && firstLine[0] != "" {
			preview := strings.Join(firstLine, "\n")
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			lines = append(lines, "output: "+preview)
		}
		return ToolRenderResult{
			Summary:     result.Summary,
			DetailLines: lines,
			Status:      status,
			CompactLine: fmt.Sprintf("passed=%d failed=%d skipped=%d (%.1fs)", result.Passed, result.Failed, result.Skipped, result.ElapsedS),
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
