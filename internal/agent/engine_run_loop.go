package agent

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
)

func (e *defaultEngine) runPromptTurns(ctx context.Context, sess *session.Session, setup runPromptSetup, out io.Writer) (string, error) {
	if e == nil || e.runner == nil {
		return "", fmt.Errorf("agent engine is unavailable")
	}

	runner := e.runner
	toolSequenceTracker := NewToolSequenceTracker(DefaultRepeatedToolSequenceThreshold)
	adaptiveState := newAdaptiveTurnState(runner.contextBudgetMaxReactiveRetry())
	executedToolNames := make([]string, 0, 16)
	taskReport := &TaskReport{}
	writeSystemSandboxStartupNotice(out, setup, runner.config.SandboxEnabled, runner.config.SystemSandboxMode)
	appendSystemSandboxStartupAudit(ctx, runner, sess, setup, runner.config.SandboxEnabled, runner.config.SystemSandboxMode)
	recordSystemSandboxStartupFallback(taskReport, setup, runner.config.SystemSandboxMode)
	approvalHandler := runner.prepareRunApprovalHandler(setup, out)
	sandboxAudit := sandboxAuditFromSetup(setup, runner.config.SandboxEnabled, runner.config.SystemSandboxMode)

	for step := 0; step < runner.config.MaxIterations; step++ {
		messages, err := e.messagesForStep(ctx, sess, setup, step, out)
		if err != nil {
			return "", err
		}
		if note := adaptiveState.consumePendingControlNote(); note != "" {
			noteMessage := llm.NewUserTextMessage(note)
			if err := llm.ValidateMessage(noteMessage); err != nil {
				return "", err
			}
			messages = append(messages, noteMessage)
		}
		answer, finished, err := e.processTurnWithReactiveCompaction(ctx, setup, turnProcessParams{
			Session:          sess,
			RunMode:          setup.RunMode,
			Messages:         messages,
			Assets:           setup.Input.Assets,
			AllowedToolNames: setup.AllowedToolNames,
			DeniedToolNames:  setup.DeniedToolNames,
			AllowedTools:     setup.AllowedTools,
			DeniedTools:      setup.DeniedTools,
			SequenceTracker:  toolSequenceTracker,
			AdaptiveState:    adaptiveState,
			ExecutedTools:    &executedToolNames,
			Approval:         approvalHandler,
			SandboxAudit:     sandboxAudit,
			TaskReport:       taskReport,
			Out:              out,
		})
		if err != nil {
			return "", appendTaskReportToError(err, taskReport)
		}
		if finished {
			writeCompletionTaskReport(out, taskReport)
			return answer, nil
		}
	}

	summary := BuildStopSummary(StopSummaryInput{
		SessionID:     corepkg.SessionID(sess.ID),
		Reason:        fmt.Sprintf("I reached the current execution budget of %d turns before producing a final answer.", runner.config.MaxIterations),
		ExecutedTools: executedToolNames,
		TaskReport:    taskReport,
	})
	return e.finishWithSummary(sess, summary, out, false)
}

func (e *defaultEngine) processTurnWithReactiveCompaction(ctx context.Context, setup runPromptSetup, params turnProcessParams) (string, bool, error) {
	if e == nil || e.runner == nil {
		return "", false, fmt.Errorf("agent engine is unavailable")
	}

	runner := e.runner
	maxRetry := runner.contextBudgetMaxReactiveRetry()
	for attempt := 0; ; attempt++ {
		answer, finished, err := e.processTurn(ctx, params)
		if err == nil || !isPromptTooLongError(err) {
			return answer, finished, err
		}
		if attempt >= maxRetry {
			return "", false, err
		}

		_, compacted, compactErr := runner.compactSession(ctx, params.Session, true, true, "reactive_prompt_too_long")
		if compactErr != nil {
			return "", false, compactErr
		}
		if !compacted {
			return "", false, err
		}
		if params.Out != nil {
			fmt.Fprintf(params.Out, "%scontext exceeded model window; compacted and retrying (%d/%d)%s\n", ansiDim, attempt+1, maxRetry, ansiReset)
		}

		retryMessages, buildErr := e.buildTurnMessages(params.Session, setup)
		if buildErr != nil {
			return "", false, buildErr
		}
		params.Messages = retryMessages
	}
}

func (e *defaultEngine) messagesForStep(ctx context.Context, sess *session.Session, setup runPromptSetup, step int, out io.Writer) ([]llm.Message, error) {
	if e == nil || e.runner == nil {
		return nil, fmt.Errorf("agent engine is unavailable")
	}

	runner := e.runner
	messages, err := e.buildTurnMessages(sess, setup)
	if err != nil {
		return nil, err
	}
	if step != 0 {
		return messages, nil
	}

	requestTokens := contextpkg.EstimateRequestTokens(messages)
	compacted, compactErr := runner.maybeAutoCompactSession(ctx, sess, setup.PromptTokens, requestTokens)
	if compactErr != nil {
		return nil, compactErr
	}
	if !compacted {
		return messages, nil
	}
	if out != nil {
		fmt.Fprintf(out, "%scontext compacted to fit long-history budget%s\n", ansiDim, ansiReset)
	}
	return e.buildTurnMessages(sess, setup)
}

func appendTaskReportToError(err error, taskReport *TaskReport) error {
	if err == nil || taskReport == nil || taskReport.IsEmpty() {
		return err
	}
	human := strings.TrimSpace(taskReport.HumanSummary())
	if human != "" {
		return fmt.Errorf("%w\nTask report summary:\n%s\nTask report (json):\n%s", err, human, taskReport.JSON())
	}
	return fmt.Errorf("%w\nTask report (json):\n%s", err, taskReport.JSON())
}

func writeCompletionTaskReport(out io.Writer, taskReport *TaskReport) {
	if out == nil || taskReport == nil || !taskReport.HasNonSuccessOutcomes() {
		return
	}
	human := strings.TrimSpace(taskReport.HumanSummary())
	if human == "" {
		return
	}
	_, _ = io.WriteString(out, "\nTask report summary:\n")
	_, _ = io.WriteString(out, human+"\n")
	_, _ = io.WriteString(out, "Task report (json):\n")
	_, _ = io.WriteString(out, taskReport.JSON()+"\n")
}

func writeSystemSandboxStartupNotice(out io.Writer, setup runPromptSetup, sandboxEnabled bool, configuredMode string) {
	if out == nil {
		return
	}
	mode := strings.TrimSpace(configuredMode)
	if mode == "" {
		mode = "off"
	}
	backend := strings.TrimSpace(setup.SystemSandboxBackend)
	if backend == "" {
		backend = "none"
	}
	status := strings.TrimSpace(setup.SystemSandboxStatus)
	if !sandboxEnabled && status == "" {
		return
	}
	if mode == "off" && !setup.SystemSandboxFallback && status == "" {
		return
	}
	sandboxState := "active"
	if setup.SystemSandboxFallback {
		sandboxState = "fallback"
	} else if backend == "none" {
		sandboxState = "inactive"
	}
	requiredCapable := strconv.FormatBool(setup.SystemSandboxRequiredCapable)
	shellNetwork := strconv.FormatBool(setup.SystemSandboxShellNetwork)
	workerNetwork := strconv.FormatBool(setup.SystemSandboxWorkerNetwork)
	capabilityLevel := strings.TrimSpace(setup.SystemSandboxCapabilityLevel)
	if capabilityLevel == "" {
		capabilityLevel = "none"
	}
	line := fmt.Sprintf(
		"%ssystem sandbox startup%s mode=%s backend=%s state=%s required_capable=%s capability_level=%s shell_network_isolation=%s worker_network_isolation=%s",
		ansiDim, ansiReset, mode, backend, sandboxState, requiredCapable, capabilityLevel, shellNetwork, workerNetwork,
	)
	if status != "" {
		line += fmt.Sprintf(" (%s)", status)
	}
	_, _ = io.WriteString(out, line+"\n")
}

func recordSystemSandboxStartupFallback(taskReport *TaskReport, setup runPromptSetup, configuredMode string) {
	if taskReport == nil || !setup.SystemSandboxFallback {
		return
	}
	mode := strings.TrimSpace(configuredMode)
	if mode == "" {
		mode = "off"
	}
	backend := strings.TrimSpace(setup.SystemSandboxBackend)
	if backend == "" {
		backend = "none"
	}
	requiredCapable := strconv.FormatBool(setup.SystemSandboxRequiredCapable)
	shellNetwork := strconv.FormatBool(setup.SystemSandboxShellNetwork)
	workerNetwork := strconv.FormatBool(setup.SystemSandboxWorkerNetwork)
	capabilityLevel := strings.TrimSpace(setup.SystemSandboxCapabilityLevel)
	if capabilityLevel == "" {
		capabilityLevel = "none"
	}
	reason := strings.TrimSpace(setup.SystemSandboxStatus)
	note := fmt.Sprintf(
		"startup (mode=%s, backend=%s, required_capable=%s, capability_level=%s, shell_network_isolation=%s, worker_network_isolation=%s",
		mode, backend, requiredCapable, capabilityLevel, shellNetwork, workerNetwork,
	)
	if reason != "" {
		note += fmt.Sprintf(", reason=%s", reason)
	}
	note += ")"
	taskReport.RecordSystemSandboxFallback(note)
}

func appendSystemSandboxStartupAudit(
	ctx context.Context,
	runner *Runner,
	sess *session.Session,
	setup runPromptSetup,
	sandboxEnabled bool,
	configuredMode string,
) {
	if runner == nil || sess == nil {
		return
	}
	mode := strings.TrimSpace(configuredMode)
	if mode == "" {
		mode = "off"
	}
	backend := strings.TrimSpace(setup.SystemSandboxBackend)
	if backend == "" {
		backend = "none"
	}
	reason := strings.TrimSpace(setup.SystemSandboxStatus)
	if !sandboxEnabled && mode == "off" && reason == "" {
		return
	}
	state := "active"
	if setup.SystemSandboxFallback {
		state = "fallback"
	} else if backend == "none" {
		state = "inactive"
	}
	metadata := map[string]string{
		"sandbox_enabled":                  strconv.FormatBool(sandboxEnabled),
		"sandbox_mode":                     mode,
		"sandbox_backend":                  backend,
		"sandbox_required_capable":         strconv.FormatBool(setup.SystemSandboxRequiredCapable),
		"sandbox_capability_level":         capabilityLevelFromSetup(setup),
		"sandbox_shell_network_isolation":  strconv.FormatBool(setup.SystemSandboxShellNetwork),
		"sandbox_worker_network_isolation": strconv.FormatBool(setup.SystemSandboxWorkerNetwork),
		"sandbox_status":                   state,
		"sandbox_fallback":                 strconv.FormatBool(setup.SystemSandboxFallback),
	}
	if reason != "" {
		metadata["sandbox_message"] = reason
	}
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
		Actor:     "agent",
		Action:    "system_sandbox_startup",
		Result:    state,
		Metadata:  metadata,
	})
}

func capabilityLevelFromSetup(setup runPromptSetup) string {
	level := strings.TrimSpace(setup.SystemSandboxCapabilityLevel)
	if level == "" {
		return "none"
	}
	return level
}
