package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

const repeatedToolSequenceThreshold = 3

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiCyan    = "\x1b[36m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiRed     = "\x1b[31m"
	toolPreview = 3
)

type Options struct {
	Workspace string
	Config    config.Config
	Client    llm.Client
	Store     *session.Store
	Registry  *tools.Registry
	Observer  Observer
	Approval  tools.ApprovalHandler
	Stdin     io.Reader
	Stdout    io.Writer
}

type Runner struct {
	workspace string
	config    config.Config
	client    llm.Client
	store     *session.Store
	registry  *tools.Registry
	observer  Observer
	approval  tools.ApprovalHandler
	stdin     io.Reader
	stdout    io.Writer
}

func NewRunner(opts Options) *Runner {
	return &Runner{
		workspace: opts.Workspace,
		config:    opts.Config,
		client:    opts.Client,
		store:     opts.Store,
		registry:  opts.Registry,
		observer:  opts.Observer,
		approval:  opts.Approval,
		stdin:     opts.Stdin,
		stdout:    opts.Stdout,
	}
}

func (r *Runner) SetObserver(observer Observer) {
	r.observer = observer
}

func (r *Runner) SetApprovalHandler(handler tools.ApprovalHandler) {
	r.approval = handler
}

func (r *Runner) RunPrompt(ctx context.Context, sess *session.Session, userInput string, out io.Writer) (string, error) {
	sess.Messages = append(sess.Messages, llm.Message{
		Role:    "user",
		Content: userInput,
	})
	if err := r.store.Save(sess); err != nil {
		return "", err
	}
	r.emit(Event{
		Type:      EventRunStarted,
		SessionID: sess.ID,
		UserInput: userInput,
	})

	lastToolSequenceSignature := ""
	repeatedToolSequenceCount := 0
	executedToolNames := make([]string, 0, 16)

	for step := 0; step < r.config.MaxIterations; step++ {
		messages := make([]llm.Message, 0, len(sess.Messages)+1)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: systemPrompt(r.workspace, r.config.ApprovalPolicy),
		})
		messages = append(messages, sess.Messages...)

		request := llm.ChatRequest{
			Model:       r.config.Provider.Model,
			Messages:    messages,
			Tools:       r.registry.Definitions(),
			Temperature: 0.2,
		}

		streamedText := false
		reply, err := r.completeTurn(ctx, request, out, &streamedText)
		if err != nil {
			return "", err
		}

		if len(reply.ToolCalls) == 0 {
			sess.Messages = append(sess.Messages, reply)
			if err := r.store.Save(sess); err != nil {
				return "", err
			}
			r.emit(Event{
				Type:      EventAssistantMessage,
				SessionID: sess.ID,
				Content:   reply.Content,
			})
			r.emit(Event{
				Type:      EventRunFinished,
				SessionID: sess.ID,
				Content:   reply.Content,
			})

			answer := strings.TrimSpace(reply.Content)
			if answer == "" {
				return "", fmt.Errorf("assistant returned neither content nor tool calls")
			}
			if out != nil && !streamedText {
				fmt.Fprintln(out)
				fmt.Fprintln(out, answer)
			}
			return answer, nil
		}

		toolSequenceSignature := signatureToolCalls(reply.ToolCalls)
		if toolSequenceSignature == lastToolSequenceSignature {
			repeatedToolSequenceCount++
		} else {
			lastToolSequenceSignature = toolSequenceSignature
			repeatedToolSequenceCount = 1
		}
		if repeatedToolSequenceCount >= repeatedToolSequenceThreshold {
			summary := r.buildStopSummary(
				sess,
				fmt.Sprintf("I stopped because the assistant repeated the same tool sequence %d times in a row (%s).", repeatedToolSequenceCount, strings.Join(uniqueToolCallNames(reply.ToolCalls), ", ")),
				executedToolNames,
			)
			return r.finishWithSummary(sess, summary, out, streamedText)
		}

		sess.Messages = append(sess.Messages, reply)
		if err := r.store.Save(sess); err != nil {
			return "", err
		}

		if streamedText && out != nil {
			fmt.Fprintln(out)
		}
		for _, call := range reply.ToolCalls {
			executedToolNames = append(executedToolNames, call.Function.Name)
			r.emit(Event{
				Type:          EventToolCallStarted,
				SessionID:     sess.ID,
				ToolName:      call.Function.Name,
				ToolArguments: call.Function.Arguments,
			})
			if out != nil {
				fmt.Fprintf(out, "%s%stool>%s %s\n", ansiBold, ansiCyan, ansiReset, call.Function.Name)
			}

			result, execErr := r.registry.Execute(ctx, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
				Workspace:      r.workspace,
				ApprovalPolicy: r.config.ApprovalPolicy,
				Approval:       r.approval,
				Session:        sess,
				Stdin:          r.stdin,
				Stdout:         r.stdout,
			})
			if execErr != nil {
				result = marshalToolResult(map[string]any{
					"ok":    false,
					"error": execErr.Error(),
				})
			}
			if out != nil {
				r.renderToolFeedback(out, call.Function.Name, result)
			}
			errText := ""
			if execErr != nil {
				errText = execErr.Error()
			}
			r.emit(Event{
				Type:       EventToolCallCompleted,
				SessionID:  sess.ID,
				ToolName:   call.Function.Name,
				ToolResult: result,
				Error:      errText,
			})

			sess.Messages = append(sess.Messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
			if err := r.store.Save(sess); err != nil {
				return "", err
			}
			if call.Function.Name == "update_plan" {
				r.emit(Event{
					Type:      EventPlanUpdated,
					SessionID: sess.ID,
					Plan:      append([]session.PlanItem(nil), sess.Plan...),
				})
			}
		}
	}

	summary := r.buildStopSummary(
		sess,
		fmt.Sprintf("I reached the current execution budget of %d turns before producing a final answer.", r.config.MaxIterations),
		executedToolNames,
	)
	return r.finishWithSummary(sess, summary, out, false)
}

func (r *Runner) completeTurn(ctx context.Context, request llm.ChatRequest, out io.Writer, streamedText *bool) (llm.Message, error) {
	if !r.config.Stream {
		return r.client.CreateMessage(ctx, request)
	}

	return r.client.StreamMessage(ctx, request, func(delta string) {
		if out == nil || delta == "" {
			if delta != "" {
				r.emit(Event{Type: EventAssistantDelta, Content: delta})
			}
			return
		}
		if !*streamedText {
			fmt.Fprintln(out)
		}
		*streamedText = true
		fmt.Fprint(out, delta)
		r.emit(Event{Type: EventAssistantDelta, Content: delta})
	})
}

func (r *Runner) renderToolFeedback(out io.Writer, name, payload string) {
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		fmt.Fprintf(out, "  %serror%s %s\n\n", ansiRed, ansiReset, envelope.Error)
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
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %slisted%s %d entries under %s\n", ansiGreen, ansiReset, len(result.Items), emptyDot(result.Root))
			for _, item := range previewPaths(result.Items) {
				fmt.Fprintf(out, "    %s\n", item)
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
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			fmt.Fprintf(out, "  %sfound%s %d matches for %q\n", ansiGreen, ansiReset, len(result.Matches), result.Query)
			for _, match := range previewMatches(result.Matches) {
				fmt.Fprintf(out, "    %s\n", match)
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
			OK       bool   `json:"ok"`
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if err := json.Unmarshal([]byte(payload), &result); err == nil {
			statusColor := ansiGreen
			if !result.OK {
				statusColor = ansiYellow
			}
			fmt.Fprintf(out, "  %sexit%s code %d\n", statusColor, ansiReset, result.ExitCode)
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

func (r *Runner) finishWithSummary(sess *session.Session, summary string, out io.Writer, streamedText bool) (string, error) {
	sess.Messages = append(sess.Messages, llm.Message{
		Role:    "assistant",
		Content: summary,
	})
	if err := r.store.Save(sess); err != nil {
		return "", err
	}
	r.emit(Event{
		Type:      EventAssistantMessage,
		SessionID: sess.ID,
		Content:   summary,
	})
	r.emit(Event{
		Type:      EventRunFinished,
		SessionID: sess.ID,
		Content:   summary,
	})
	if out != nil {
		if streamedText {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, summary)
	}
	return summary, nil
}

func (r *Runner) buildStopSummary(sess *session.Session, reason string, executedToolNames []string) string {
	var builder strings.Builder
	builder.WriteString("Paused before a final answer.\n")
	builder.WriteString(reason)

	recentTools := recentToolNames(executedToolNames, 4)
	if len(recentTools) > 0 {
		builder.WriteString("\nRecent tool activity:\n")
		for _, toolName := range recentTools {
			fmt.Fprintf(&builder, "- %s\n", toolName)
		}
	}

	fmt.Fprintf(&builder, "\nYou can continue by reusing session %s with -session %s, or raise the budget with -max-iterations <n>.", sess.ID, sess.ID)
	return builder.String()
}

func signatureToolCalls(calls []llm.ToolCall) string {
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, call.Function.Name+":"+normalizeToolArguments(call.Function.Arguments))
	}
	return strings.Join(parts, "|")
}

func normalizeToolArguments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	data, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(data)
}

func uniqueToolCallNames(calls []llm.ToolCall) []string {
	seen := make(map[string]struct{}, len(calls))
	result := make([]string, 0, len(calls))
	for _, call := range calls {
		if _, ok := seen[call.Function.Name]; ok {
			continue
		}
		seen[call.Function.Name] = struct{}{}
		result = append(result, call.Function.Name)
	}
	return result
}

func recentToolNames(names []string, limit int) []string {
	if limit <= 0 || len(names) == 0 {
		return nil
	}
	result := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for i := len(names) - 1; i >= 0 && len(result) < limit; i-- {
		name := names[i]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func marshalToolResult(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"failed to encode tool result"}`
	}
	return string(data)
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

func (r *Runner) emit(event Event) {
	if r.observer == nil {
		return
	}
	r.observer.HandleEvent(event)
}
