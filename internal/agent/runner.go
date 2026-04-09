package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/skills"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
)

const repeatedToolSequenceThreshold = 3
const (
	maxActiveSkillDescriptionChars  = 320
	maxActiveSkillInstructionsChars = 3600
	emptyAllowlistSentinel          = "__bytemind__no_tools__"
	emptyReplyFallback              = "Model returned an empty response (no text and no tool calls). Retry the request or switch model if this persists."
)

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
	Workspace    string
	Config       config.Config
	Client       llm.Client
	Store        *session.Store
	Registry     *tools.Registry
	SkillManager *skills.Manager
	TokenManager *tokenusage.TokenUsageManager
	Observer     Observer
	Approval     tools.ApprovalHandler
	Stdin        io.Reader
	Stdout       io.Writer
}

type RunPromptInput struct {
	UserMessage llm.Message
	Assets      map[llm.AssetID]llm.ImageAsset
	DisplayText string
}

type Runner struct {
	workspace    string
	config       config.Config
	client       llm.Client
	store        *session.Store
	registry     *tools.Registry
	skillManager *skills.Manager
	tokenManager *tokenusage.TokenUsageManager
	observer     Observer
	approval     tools.ApprovalHandler
	stdin        io.Reader
	stdout       io.Writer
}

type TokenRealtimeSnapshot struct {
	SessionID            string
	SessionInputTokens   int64
	SessionOutputTokens  int64
	SessionContextTokens int64
	SessionTotalTokens   int64
	GlobalTotalTokens    int64
	CurrentTPS           float64
	PeakTPS              float64
	ActiveSessions       int
	ErrorRate            float64
	AvgLatency           time.Duration
	GeneratedAt          time.Time
}

func NewRunner(opts Options) *Runner {
	manager := opts.SkillManager
	if manager == nil {
		manager = skills.NewManager(opts.Workspace)
	}
	return &Runner{
		workspace:    opts.Workspace,
		config:       opts.Config,
		client:       opts.Client,
		store:        opts.Store,
		registry:     opts.Registry,
		skillManager: manager,
		tokenManager: opts.TokenManager,
		observer:     opts.Observer,
		approval:     opts.Approval,
		stdin:        opts.Stdin,
		stdout:       opts.Stdout,
	}
}

func (r *Runner) SetObserver(observer Observer) {
	r.observer = observer
}

func (r *Runner) SetApprovalHandler(handler tools.ApprovalHandler) {
	r.approval = handler
}

func (r *Runner) HasTokenManager() bool {
	return r != nil && r.tokenManager != nil
}

func (r *Runner) TokenRealtimeEnabled() bool {
	return r != nil && r.tokenManager != nil && r.config.TokenUsage.EnableRealtime
}

func (r *Runner) GetTokenRealtimeSnapshot(sessionID string) (TokenRealtimeSnapshot, error) {
	var snapshot TokenRealtimeSnapshot
	if r == nil || r.tokenManager == nil {
		return snapshot, fmt.Errorf("token manager unavailable")
	}
	realtime, err := r.tokenManager.GetRealtimeStats()
	if err != nil {
		return snapshot, err
	}
	snapshot.GlobalTotalTokens = realtime.TotalTokens
	snapshot.CurrentTPS = realtime.Metrics.CurrentTPS
	snapshot.PeakTPS = realtime.Metrics.PeakTPS
	snapshot.ActiveSessions = realtime.Metrics.ActiveSessions
	snapshot.ErrorRate = realtime.Metrics.ErrorRate
	snapshot.AvgLatency = realtime.Metrics.Latency
	snapshot.GeneratedAt = realtime.GeneratedAt
	snapshot.SessionID = strings.TrimSpace(sessionID)

	if snapshot.SessionID != "" {
		for _, stats := range realtime.Sessions {
			if stats == nil || stats.SessionID != snapshot.SessionID {
				continue
			}
			snapshot.SessionInputTokens = stats.InputTokens
			snapshot.SessionOutputTokens = stats.OutputTokens
			snapshot.SessionTotalTokens = stats.TotalTokens
			break
		}
	}
	return snapshot, nil
}

func (r *Runner) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	if r.skillManager == nil {
		return nil, nil
	}
	return r.skillManager.List()
}

func (r *Runner) ActivateSkill(sess *session.Session, name string, args map[string]string) (skills.Skill, error) {
	if sess == nil {
		return skills.Skill{}, fmt.Errorf("session is required")
	}
	if r.skillManager == nil {
		return skills.Skill{}, fmt.Errorf("skill manager is unavailable")
	}
	skill, ok := r.skillManager.Find(name)
	if !ok {
		return skills.Skill{}, fmt.Errorf("skill not found: %s", strings.TrimSpace(name))
	}

	normalizedArgs := normalizeSkillArgs(args)
	for _, arg := range skill.Args {
		if _, exists := normalizedArgs[arg.Name]; !exists && strings.TrimSpace(arg.Default) != "" {
			normalizedArgs[arg.Name] = strings.TrimSpace(arg.Default)
		}
		if arg.Required && strings.TrimSpace(normalizedArgs[arg.Name]) == "" {
			return skills.Skill{}, fmt.Errorf("missing required skill arg: %s", arg.Name)
		}
	}
	if len(normalizedArgs) == 0 {
		normalizedArgs = nil
	}

	sess.ActiveSkill = &session.ActiveSkill{
		Name:        skill.Name,
		Args:        normalizedArgs,
		ActivatedAt: time.Now().UTC(),
	}
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return skills.Skill{}, err
		}
	}
	return skill, nil
}

func (r *Runner) ClearActiveSkill(sess *session.Session) error {
	if sess == nil {
		return fmt.Errorf("session is required")
	}
	sess.ActiveSkill = nil
	if r.store != nil {
		return r.store.Save(sess)
	}
	return nil
}

func (r *Runner) GetActiveSkill(sess *session.Session) (skills.Skill, bool) {
	if sess == nil || sess.ActiveSkill == nil || r.skillManager == nil {
		return skills.Skill{}, false
	}
	return r.skillManager.Find(sess.ActiveSkill.Name)
}

func (r *Runner) UpdateProvider(providerCfg config.ProviderConfig, client llm.Client) {
	r.config.Provider = providerCfg
	if client != nil {
		r.client = client
	}
}

func (r *Runner) RunPrompt(ctx context.Context, sess *session.Session, userInput, mode string, out io.Writer) (string, error) {
	return r.RunPromptWithInput(ctx, sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage(userInput),
		DisplayText: userInput,
	}, mode, out)
}

func (r *Runner) RunPromptWithInput(ctx context.Context, sess *session.Session, input RunPromptInput, mode string, out io.Writer) (string, error) {
	userMessage := input.UserMessage
	userMessage.Normalize()
	if userMessage.Role == "" {
		userMessage = llm.NewUserTextMessage(input.DisplayText)
	}
	if strings.TrimSpace(input.DisplayText) == "" {
		input.DisplayText = userMessage.Text()
	}
	userInput := input.DisplayText

	runMode := planpkg.NormalizeMode(mode)
	if strings.TrimSpace(mode) == "" {
		runMode = planpkg.NormalizeMode(string(sess.Mode))
	}
	mode = string(runMode)
	if sess.Mode != runMode {
		sess.Mode = runMode
	}
	if runMode == planpkg.ModePlan {
		goalText := strings.TrimSpace(userInput)
		if goalText == "" {
			goalText = strings.TrimSpace(userMessage.Text())
		}
		if strings.TrimSpace(sess.Plan.Goal) == "" {
			sess.Plan.Goal = goalText
		}
		if sess.Plan.Phase == planpkg.PhaseNone {
			sess.Plan.Phase = planpkg.PhaseDrafting
		}
	}

	if err := llm.ValidateMessage(userMessage); err != nil {
		return "", err
	}
	sess.Messages = append(sess.Messages, userMessage)
	if err := r.store.Save(sess); err != nil {
		return "", err
	}
	_ = history.AppendPrompt(r.workspace, sess.ID, userInput, time.Now().UTC())
	r.emit(Event{
		Type:      EventRunStarted,
		SessionID: sess.ID,
		UserInput: userInput,
	})

	activeSkill := r.resolveActiveSkill(sess)
	allowedTools, deniedTools := policySets(activeSkill)
	allowedToolNames := sortedToolNames(allowedTools)
	deniedToolNames := sortedToolNames(deniedTools)

	lastToolSequenceSignature := ""
	repeatedToolSequenceCount := 0
	executedToolNames := make([]string, 0, 16)
	availableSkills := r.promptSkills()
	availableTools := toolNames(r.registry.DefinitionsForMode(runMode))
	instructionText := loadAGENTSInstruction(r.workspace)
	webLookupInstruction := explicitWebLookupInstruction(userInput)

	for step := 0; step < r.config.MaxIterations; step++ {
		messages := make([]llm.Message, 0, len(sess.Messages)+2)
		systemMessage := llm.NewTextMessage(llm.RoleSystem, systemPrompt(PromptInput{
			Workspace:      r.workspace,
			ApprovalPolicy: r.config.ApprovalPolicy,
			Model:          r.config.Provider.Model,
			Mode:           mode,
			Skills:         availableSkills,
			Tools:          availableTools,
			ActiveSkill:    promptActiveSkill(activeSkill),
			Instruction:    instructionText,
		}))
		if err := llm.ValidateMessage(systemMessage); err != nil {
			return "", err
		}
		messages = append(messages, systemMessage)
		if webLookupInstruction != "" {
			webLookupMessage := llm.NewTextMessage(llm.RoleSystem, webLookupInstruction)
			if err := llm.ValidateMessage(webLookupMessage); err != nil {
				return "", err
			}
			messages = append(messages, webLookupMessage)
		}
		messages = append(messages, sess.Messages...)

		filteredTools := r.registry.DefinitionsForModeWithFilters(runMode, allowedToolNames, deniedToolNames)
		caps := llm.DefaultModelCapabilities.Resolve(r.config.Provider.Model)
		requestMessages := llm.ApplyCapabilities(messages, caps)
		requestTools := filteredTools
		if !caps.SupportsToolUse {
			requestTools = nil
		}

		request := llm.ChatRequest{
			Model:       r.config.Provider.Model,
			Messages:    requestMessages,
			Tools:       requestTools,
			Assets:      input.Assets,
			Temperature: 0.2,
		}

		streamedText := false
		turnStart := time.Now()
		reply, err := r.completeTurn(ctx, request, out, &streamedText)
		turnLatency := time.Since(turnStart)
		if err != nil {
			estimatedUsage := r.resolveTurnUsage(request, nil)
			r.recordTokenUsage(ctx, sess, request, estimatedUsage, turnLatency, false)
			return "", err
		}
		reply.Normalize()
		turnUsage := r.resolveTurnUsage(request, &reply)
		r.recordTokenUsage(ctx, sess, request, turnUsage, turnLatency, true)
		r.emitUsageEvent(sess, &turnUsage)

		if len(reply.ToolCalls) == 0 {
			answer := strings.TrimSpace(reply.Content)
			if answer == "" {
				reply.Content = emptyReplyFallback
				answer = emptyReplyFallback
			}
			if runMode == planpkg.ModePlan && !planpkg.HasStructuredPlan(sess.Plan) {
				reminder := "Plan mode requires a structured plan before finishing. Please restate the plan using update_plan."
				if answer != "" {
					answer += "\n\n" + reminder
				} else {
					answer = reminder
				}
				reply = llm.NewAssistantTextMessage(answer)
			}
			if err := llm.ValidateMessage(reply); err != nil {
				return "", err
			}
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

			answer = strings.TrimSpace(reply.Content)
			if out != nil && !streamedText {
				fmt.Fprintln(out)
				fmt.Fprintln(out, answer)
			}
			return answer, nil
		}

		if err := llm.ValidateMessage(reply); err != nil {
			return "", err
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

			result, execErr := r.registry.ExecuteForMode(ctx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
				Workspace:      r.workspace,
				ApprovalPolicy: r.config.ApprovalPolicy,
				Approval:       r.approval,
				Session:        sess,
				Mode:           runMode,
				Stdin:          r.stdin,
				Stdout:         r.stdout,
				AllowedTools:   allowedTools,
				DeniedTools:    deniedTools,
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

			toolMessage := llm.NewToolResultMessage(call.ID, result)
			if err := llm.ValidateMessage(toolMessage); err != nil {
				return "", err
			}
			sess.Messages = append(sess.Messages, toolMessage)
			if err := r.store.Save(sess); err != nil {
				return "", err
			}
			if call.Function.Name == "update_plan" {
				r.emit(Event{
					Type:      EventPlanUpdated,
					SessionID: sess.ID,
					Plan:      planpkg.CloneState(sess.Plan),
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

func (r *Runner) emitUsageEvent(sess *session.Session, usage *llm.Usage) {
	if sess == nil || usage == nil {
		return
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ContextTokens == 0 && usage.TotalTokens == 0 {
		return
	}
	r.emit(Event{
		Type:      EventUsageUpdated,
		SessionID: sess.ID,
		Usage:     *usage,
	})
}

func (r *Runner) recordTokenUsage(ctx context.Context, sess *session.Session, request llm.ChatRequest, usage llm.Usage, latency time.Duration, success bool) {
	if r.tokenManager == nil || sess == nil {
		return
	}

	req := &tokenusage.TokenRecordRequest{
		SessionID:    sess.ID,
		ModelName:    request.Model,
		InputTokens:  int64(max(0, usage.InputTokens+usage.ContextTokens)),
		OutputTokens: int64(max(0, usage.OutputTokens)),
		RequestID:    time.Now().UTC().Format("20060102150405.000000000"),
		Latency:      latency,
		Success:      success,
		Metadata: map[string]string{
			"workspace": sess.Workspace,
		},
	}
	if err := r.tokenManager.RecordTokenUsage(ctx, req); err != nil && r.stdout != nil {
		fmt.Fprintf(r.stdout, "%swarning%s token usage record failed: %v\n", ansiDim, ansiReset, err)
	}
}

func (r *Runner) resolveTurnUsage(request llm.ChatRequest, reply *llm.Message) llm.Usage {
	if reply != nil && reply.Usage != nil {
		usage := *reply.Usage
		input := max(0, usage.InputTokens)
		output := max(0, usage.OutputTokens)
		context := max(0, usage.ContextTokens)
		total := usage.TotalTokens
		if total <= 0 {
			total = input + output + context
		}
		return llm.Usage{
			InputTokens:   input,
			OutputTokens:  output,
			ContextTokens: context,
			TotalTokens:   max(0, total),
		}
	}

	input := int(tokenusage.ApproximateRequestTokens(request.Messages))
	output := 0
	if reply != nil {
		output += int(tokenusage.ApproximateTokens(reply.Content))
		for _, call := range reply.ToolCalls {
			output += int(tokenusage.ApproximateTokens(call.Function.Name))
			output += int(tokenusage.ApproximateTokens(call.Function.Arguments))
		}
	}
	total := input + output
	return llm.Usage{
		InputTokens:   max(0, input),
		OutputTokens:  max(0, output),
		ContextTokens: 0,
		TotalTokens:   max(0, total),
	}
}

func (r *Runner) Close() error {
	if r == nil || r.tokenManager == nil {
		return nil
	}
	return r.tokenManager.Close()
}

func (r *Runner) completeTurn(ctx context.Context, request llm.ChatRequest, out io.Writer, streamedText *bool) (llm.Message, error) {
	if !r.config.Stream {
		return r.client.CreateMessage(ctx, request)
	}

	reply, err := r.client.StreamMessage(ctx, request, func(delta string) {
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
	if err != nil {
		return llm.Message{}, err
	}
	if strings.TrimSpace(reply.Content) != "" || len(reply.ToolCalls) > 0 {
		return reply, nil
	}

	// Some providers/models occasionally return empty streaming payloads while
	// still producing a valid non-stream completion. Retry once without stream.
	fallback, fallbackErr := r.client.CreateMessage(ctx, request)
	if fallbackErr == nil {
		return fallback, nil
	}
	return reply, nil
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
	summaryMessage := llm.NewAssistantTextMessage(summary)
	if err := llm.ValidateMessage(summaryMessage); err != nil {
		return "", err
	}
	sess.Messages = append(sess.Messages, summaryMessage)
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

type activeSkillRuntime struct {
	Skill skills.Skill
	Args  map[string]string
}

func (r *Runner) resolveActiveSkill(sess *session.Session) *activeSkillRuntime {
	if sess == nil || sess.ActiveSkill == nil || r.skillManager == nil {
		return nil
	}

	skill, ok := r.skillManager.Find(sess.ActiveSkill.Name)
	if !ok {
		sess.ActiveSkill = nil
		if r.store != nil {
			_ = r.store.Save(sess)
		}
		return nil
	}

	return &activeSkillRuntime{
		Skill: skill,
		Args:  normalizeSkillArgs(sess.ActiveSkill.Args),
	}
}

func policySets(active *activeSkillRuntime) (map[string]struct{}, map[string]struct{}) {
	if active == nil {
		return nil, nil
	}
	items := active.Skill.ToolPolicy.Items
	switch active.Skill.ToolPolicy.Policy {
	case skills.ToolPolicyAllowlist:
		if len(items) == 0 {
			return map[string]struct{}{emptyAllowlistSentinel: {}}, nil
		}
		allow := toToolSet(items)
		if allow == nil {
			return map[string]struct{}{emptyAllowlistSentinel: {}}, nil
		}
		return allow, nil
	case skills.ToolPolicyDenylist:
		return nil, toToolSet(items)
	default:
		return nil, nil
	}
}

func toToolSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func sortedToolNames(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func promptActiveSkill(active *activeSkillRuntime) *PromptActiveSkill {
	if active == nil {
		return nil
	}

	instruction := strings.TrimSpace(active.Skill.Instruction)
	if instruction != "" {
		instruction = trimTextWithEllipsis(instruction, maxActiveSkillInstructionsChars)
	}
	description := trimTextWithEllipsis(strings.TrimSpace(active.Skill.Description), maxActiveSkillDescriptionChars)
	whenToUse := trimTextWithEllipsis(strings.TrimSpace(active.Skill.WhenToUse), maxActiveSkillDescriptionChars)

	return &PromptActiveSkill{
		Name:         active.Skill.Name,
		Description:  description,
		WhenToUse:    whenToUse,
		Instructions: instruction,
		Args:         normalizeSkillArgs(active.Args),
		ToolPolicy:   string(active.Skill.ToolPolicy.Policy),
		Tools:        append([]string(nil), active.Skill.ToolPolicy.Items...),
	}
}

func trimTextWithEllipsis(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func normalizeSkillArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]string, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (r *Runner) emit(event Event) {
	if r.observer == nil {
		return
	}
	r.observer.HandleEvent(event)
}

func (r *Runner) promptSkills() []PromptSkill {
	if r.skillManager == nil {
		return nil
	}
	skillList, _ := r.skillManager.List()
	if len(skillList) == 0 {
		return nil
	}
	out := make([]PromptSkill, 0, len(skillList))
	for _, skill := range skillList {
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if name == "" || description == "" {
			continue
		}
		out = append(out, PromptSkill{
			Name:        name,
			Description: description,
			Enabled:     true,
		})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func toolNames(definitions []llm.ToolDefinition) []string {
	if len(definitions) == 0 {
		return nil
	}
	names := make([]string, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		name := strings.TrimSpace(definition.Function.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func explicitWebLookupInstruction(userInput string) string {
	text := strings.ToLower(strings.TrimSpace(userInput))
	if text == "" {
		return ""
	}

	webSignals := []string{
		"github", "gitlab", "bitbucket",
		"联网", "上网", "互联网", "网上",
		"源码", "源代码", "repo", "repository",
		"official website", "官网",
	}
	matched := false
	for _, signal := range webSignals {
		if strings.Contains(text, signal) {
			matched = true
			break
		}
	}
	if !matched {
		return ""
	}

	return "The user explicitly requested online or GitHub-source lookup. Use web_search/web_fetch first. Do not substitute local-workspace tools (list_files/read_file/search_text) for this request unless the user explicitly asks to inspect the current workspace repository."
}
