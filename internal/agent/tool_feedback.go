package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func (r *Runner) renderToolFeedback(out io.Writer, name, payload string) {
	var envelope struct {
		OK            *bool  `json:"ok"`
		Error         string `json:"error"`
		Status        string `json:"status"`
		ReasonCode    string `json:"reason_code"`
		SystemSandbox struct {
			Mode            string `json:"mode"`
			Backend         string `json:"backend"`
			Active          bool   `json:"active"`
			RequiredCapable bool   `json:"required_capable"`
			CapabilityLevel string `json:"capability_level"`
			ShellNetwork    bool   `json:"shell_network_isolation"`
			WorkerNetwork   bool   `json:"worker_network_isolation"`
			Fallback        bool   `json:"fallback"`
			Reason          string `json:"fallback_reason"`
		} `json:"system_sandbox"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		status := strings.ToLower(strings.TrimSpace(envelope.Status))
		reasonCode := strings.ToLower(strings.TrimSpace(envelope.ReasonCode))
		sandboxSummary := formatSystemSandboxSummary(
			envelope.SystemSandbox.Mode,
			envelope.SystemSandbox.Backend,
			envelope.SystemSandbox.Active,
			envelope.SystemSandbox.RequiredCapable,
			envelope.SystemSandbox.CapabilityLevel,
			envelope.SystemSandbox.ShellNetwork,
			envelope.SystemSandbox.WorkerNetwork,
			envelope.SystemSandbox.Fallback,
		)
		if reasonCode == "permission_denied" || (status == "denied" && reasonCode == "") {
			fmt.Fprintf(out, "  %spending approval%s %s\n", ansiYellow, ansiReset, normalizeApprovalErrorMessage(envelope.Error, reasonCode))
			if sandboxSummary != "" {
				fmt.Fprintf(out, "    sandbox: %s\n", sandboxSummary)
			}
			if envelope.SystemSandbox.Fallback {
				reason := strings.TrimSpace(envelope.SystemSandbox.Reason)
				if reason != "" {
					fmt.Fprintf(out, "    sandbox reason: %s\n", compactWhitespace(reason, 120))
				}
			}
			fmt.Fprintln(out)
			return
		}
		if status == "denied" {
			fmt.Fprintf(out, "  %sdenied%s %s\n", ansiYellow, ansiReset, normalizeDeniedMessage(envelope.Error, reasonCode))
			if sandboxSummary != "" {
				fmt.Fprintf(out, "    sandbox: %s\n", sandboxSummary)
			}
			if envelope.SystemSandbox.Fallback {
				reason := strings.TrimSpace(envelope.SystemSandbox.Reason)
				if reason != "" {
					fmt.Fprintf(out, "    sandbox reason: %s\n", compactWhitespace(reason, 120))
				}
			}
			fmt.Fprintln(out)
			return
		}
		if status == "skipped" || reasonCode == "denied_dependency" {
			fmt.Fprintf(out, "  %sskipped%s %s\n", ansiDim, ansiReset, normalizeSkippedDependencyMessage(envelope.Error, reasonCode))
			if sandboxSummary != "" {
				fmt.Fprintf(out, "    sandbox: %s\n", sandboxSummary)
			}
			if envelope.SystemSandbox.Fallback {
				reason := strings.TrimSpace(envelope.SystemSandbox.Reason)
				if reason != "" {
					fmt.Fprintf(out, "    sandbox reason: %s\n", compactWhitespace(reason, 120))
				}
			}
			fmt.Fprintln(out)
			return
		}
		fmt.Fprintf(out, "  %serror%s %s\n", ansiRed, ansiReset, envelope.Error)
		if sandboxSummary != "" {
			fmt.Fprintf(out, "    sandbox: %s\n", sandboxSummary)
		}
		if envelope.SystemSandbox.Fallback {
			reason := strings.TrimSpace(envelope.SystemSandbox.Reason)
			if reason != "" {
				fmt.Fprintf(out, "    sandbox reason: %s\n", compactWhitespace(reason, 120))
			}
		}
		fmt.Fprintln(out)
		return
	}

	switch name {
	case "list_files":
		var result struct {
			Root  string `json:"root"`
			Items []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"items"`
			Truncated bool   `json:"truncated"`
			Reason    string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %slisted%s %d entries under %s\n", ansiGreen, ansiReset, len(result.Items), emptyDot(result.Root))
			for _, item := range previewPaths(result.Items) {
				fmt.Fprintf(out, "    %s\n", item)
			}
			if result.Truncated {
				reason := strings.TrimSpace(result.Reason)
				if reason == "" {
					reason = "visit_limit"
				}
				fmt.Fprintf(out, "    %sstopped early%s (%s); narrow path/depth for large trees\n", ansiDim, ansiReset, reason)
			}
		}
	case "read_file":
		var result struct {
			Path       string `json:"path"`
			StartLine  int    `json:"start_line"`
			EndLine    int    `json:"end_line"`
			TotalLines int    `json:"total_lines"`
			Content    string `json:"content"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			shown := 0
			if strings.TrimSpace(result.Content) != "" && result.EndLine >= result.StartLine {
				shown = result.EndLine - result.StartLine + 1
			}
			fmt.Fprintf(out, "  %sread%s %s lines %d-%d of %d (%d shown)\n", ansiGreen, ansiReset, result.Path, result.StartLine, result.EndLine, result.TotalLines, shown)
		}
	case "search_text":
		var result struct {
			Query   string `json:"query"`
			Matches []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Text string `json:"text"`
			} `json:"matches"`
			Truncated bool   `json:"truncated"`
			Reason    string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %sfound%s %d matches for %q\n", ansiGreen, ansiReset, len(result.Matches), result.Query)
			for _, match := range previewMatches(result.Matches) {
				fmt.Fprintf(out, "    %s\n", match)
			}
			if result.Truncated {
				reason := strings.TrimSpace(result.Reason)
				if reason == "" {
					reason = "scan_budget"
				}
				fmt.Fprintf(out, "    %sstopped early%s (%s); narrow the search path and retry\n", ansiDim, ansiReset, reason)
			}
		}
	case "web_search":
		var result struct {
			Query   string `json:"query"`
			Results []struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"results"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %ssearched%s web for %q (%d results)\n", ansiGreen, ansiReset, result.Query, len(result.Results))
			previewCount := toolPreview
			if len(result.Results) < previewCount {
				previewCount = len(result.Results)
			}
			for i := 0; i < previewCount; i++ {
				title := compactWhitespace(result.Results[i].Title, 64)
				if strings.TrimSpace(title) == "" {
					title = result.Results[i].URL
				}
				fmt.Fprintf(out, "    %s - %s\n", title, result.Results[i].URL)
			}
		}
	case "web_fetch":
		var result struct {
			URL        string `json:"url"`
			StatusCode int    `json:"status_code"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			Truncated  bool   `json:"truncated"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %sfetched%s %s (HTTP %d)\n", ansiGreen, ansiReset, result.URL, result.StatusCode)
			if strings.TrimSpace(result.Title) != "" {
				fmt.Fprintf(out, "    title: %s\n", compactWhitespace(result.Title, 80))
			}
			if strings.TrimSpace(result.Content) != "" {
				fmt.Fprintf(out, "    preview: %s\n", compactWhitespace(result.Content, 100))
			}
			if result.Truncated {
				fmt.Fprintf(out, "    %scontent truncated%s\n", ansiDim, ansiReset)
			}
		}
	case "write_file":
		var result struct {
			Path         string `json:"path"`
			BytesWritten int    `json:"bytes_written"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %swrote%s %s (%d bytes)\n", ansiGreen, ansiReset, result.Path, result.BytesWritten)
		}
	case "replace_in_file":
		var result struct {
			Path     string `json:"path"`
			Replaced int    `json:"replaced"`
			OldCount int    `json:"old_count"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %supdated%s %s (%d/%d matches replaced)\n", ansiGreen, ansiReset, result.Path, result.Replaced, result.OldCount)
		}
	case "run_shell":
		var result struct {
			OK            bool   `json:"ok"`
			ExitCode      int    `json:"exit_code"`
			Stdout        string `json:"stdout"`
			Stderr        string `json:"stderr"`
			SystemSandbox struct {
				Mode            string `json:"mode"`
				Backend         string `json:"backend"`
				Active          bool   `json:"active"`
				RequiredCapable bool   `json:"required_capable"`
				CapabilityLevel string `json:"capability_level"`
				ShellNetwork    bool   `json:"shell_network_isolation"`
				WorkerNetwork   bool   `json:"worker_network_isolation"`
				Fallback        bool   `json:"fallback"`
				Reason          string `json:"fallback_reason"`
			} `json:"system_sandbox"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			statusColor := ansiGreen
			if !result.OK {
				statusColor = ansiYellow
			}
			fmt.Fprintf(out, "  %sexit%s code %d\n", statusColor, ansiReset, result.ExitCode)
			if summary := formatSystemSandboxSummary(
				result.SystemSandbox.Mode,
				result.SystemSandbox.Backend,
				result.SystemSandbox.Active,
				result.SystemSandbox.RequiredCapable,
				result.SystemSandbox.CapabilityLevel,
				result.SystemSandbox.ShellNetwork,
				result.SystemSandbox.WorkerNetwork,
				result.SystemSandbox.Fallback,
			); summary != "" {
				fmt.Fprintf(out, "    sandbox: %s\n", summary)
			}
			if result.SystemSandbox.Fallback {
				reason := strings.TrimSpace(result.SystemSandbox.Reason)
				if reason != "" {
					fmt.Fprintf(out, "    sandbox reason: %s\n", compactWhitespace(reason, 120))
				}
			}
			for _, line := range previewOutput("stdout", result.Stdout) {
				fmt.Fprintf(out, "    %s\n", line)
			}
			for _, line := range previewOutput("stderr", result.Stderr) {
				fmt.Fprintf(out, "    %s\n", line)
			}
		}
	case "apply_patch":
		var result struct {
			Operations []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"operations"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %spatch%s %d operations\n", ansiGreen, ansiReset, len(result.Operations))
			for _, op := range result.Operations {
				fmt.Fprintf(out, "    %s %s\n", op.Type, op.Path)
			}
		}
	default:
		fmt.Fprintf(out, "  %scompleted%s\n", ansiDim, ansiReset)
	}
	fmt.Fprintln(out)
}

func emptyDot(path string) string {
	if strings.TrimSpace(path) == "" {
		return "."
	}
	return path
}

func previewPaths(items []struct {
	Path string `json:"path"`
	Type string `json:"type"`
}) []string {
	limit := toolPreview
	if len(items) < limit {
		limit = len(items)
	}
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		prefix := "file"
		if items[i].Type == "dir" {
			prefix = "dir "
		}
		result = append(result, prefix+" "+items[i].Path)
	}
	return result
}

func previewMatches(matches []struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}) []string {
	limit := toolPreview
	if len(matches) < limit {
		limit = len(matches)
	}
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, fmt.Sprintf("%s:%d %s", matches[i].Path, matches[i].Line, compactWhitespace(matches[i].Text, 80)))
	}
	return result
}

func previewOutput(label, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	limit := toolPreview
	if len(lines) < limit {
		limit = len(lines)
	}
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, fmt.Sprintf("%s: %s", label, compactWhitespace(lines[i], 120)))
	}
	return result
}

func compactWhitespace(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func formatSystemSandboxSummary(
	mode,
	backend string,
	active,
	requiredCapable bool,
	capabilityLevel string,
	shellNetwork,
	workerNetwork,
	fallback bool,
) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	backend = strings.TrimSpace(backend)
	capabilityLevel = strings.ToLower(strings.TrimSpace(capabilityLevel))
	if mode == "" && backend == "" && !active && !requiredCapable && capabilityLevel == "" && !fallback {
		return ""
	}
	if mode == "" {
		mode = "off"
	}
	if backend == "" {
		backend = "none"
	}
	if capabilityLevel == "" {
		capabilityLevel = "none"
	}

	state := "inactive"
	if active {
		state = "active"
	} else if fallback {
		state = "fallback"
	}
	return fmt.Sprintf(
		"%s (mode=%s, backend=%s, required_capable=%t, capability_level=%s, shell_network_isolation=%t, worker_network_isolation=%t)",
		state,
		mode,
		backend,
		requiredCapable,
		capabilityLevel,
		shellNetwork,
		workerNetwork,
	)
}

func normalizeApprovalErrorMessage(message, reasonCode string) string {
	return normalizeReasonPrefixedMessage(message, reasonCode, "approval required")
}

func normalizeSkippedDependencyMessage(message, reasonCode string) string {
	return normalizeReasonPrefixedMessage(message, reasonCode, "skipped due to denied dependency")
}

func normalizeDeniedMessage(message, reasonCode string) string {
	return normalizeReasonPrefixedMessage(message, reasonCode, "operation denied")
}

func normalizeReasonPrefixedMessage(message, reasonCode, fallback string) string {
	message = strings.TrimSpace(message)
	reasonCode = strings.ToLower(strings.TrimSpace(reasonCode))
	if reasonCode != "" {
		prefix := reasonCode + ":"
		if strings.HasPrefix(strings.ToLower(message), prefix) {
			message = strings.TrimSpace(message[len(prefix):])
		}
	}
	if message == "" {
		return fallback
	}
	return message
}
