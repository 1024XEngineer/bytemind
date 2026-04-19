package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
)

func (e *defaultEngine) runPromptTurns(ctx context.Context, sess *session.Session, setup runPromptSetup, out io.Writer) (string, error) {
	if e == nil || e.runner == nil {
		return "", fmt.Errorf("agent engine is unavailable")
	}

	runner := e.runner
	toolSequenceTracker := runtimepkg.NewToolSequenceTracker(runtimepkg.DefaultRepeatedToolSequenceThreshold)
	adaptiveState := newAdaptiveTurnState(runner.contextBudgetMaxReactiveRetry())
	executedToolNames := make([]string, 0, 16)
	taskReport := &runtimepkg.TaskReport{}
	approvalHandler := runner.prepareRunApprovalHandler(setup, out)

	for step := 0; step < runner.config.MaxIterations; step++ {
		messages, err := e.messagesForStep(ctx, sess, setup, step, out)
		if err != nil {
			return "", err
		}
		if note := adaptiveState.consumePendingSystemNote(); note != "" {
			noteMessage := llm.NewTextMessage(llm.RoleSystem, note)
			if err := llm.ValidateMessage(noteMessage); err != nil {
				return "", err
			}
			messages = append([]llm.Message{noteMessage}, messages...)
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

	summary := runtimepkg.BuildStopSummary(runtimepkg.StopSummaryInput{
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

func appendTaskReportToError(err error, taskReport *runtimepkg.TaskReport) error {
	if err == nil || taskReport == nil || taskReport.IsEmpty() {
		return err
	}
	human := strings.TrimSpace(taskReport.HumanSummary())
	if human != "" {
		return fmt.Errorf("%w\nTask report summary:\n%s\nTask report (json):\n%s", err, human, taskReport.JSON())
	}
	return fmt.Errorf("%w\nTask report (json):\n%s", err, taskReport.JSON())
}

func writeCompletionTaskReport(out io.Writer, taskReport *runtimepkg.TaskReport) {
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
