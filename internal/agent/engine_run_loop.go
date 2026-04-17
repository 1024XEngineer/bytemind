package agent

import (
	"context"
	"fmt"
	"io"

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
	executedToolNames := make([]string, 0, 16)

	for step := 0; step < runner.config.MaxIterations; step++ {
		messages, err := e.messagesForStep(ctx, sess, setup, step, out)
		if err != nil {
			return "", err
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
			ExecutedTools:    &executedToolNames,
			Out:              out,
		})
		if err != nil {
			return "", err
		}
		if finished {
			return answer, nil
		}
	}

	summary := runtimepkg.BuildStopSummary(runtimepkg.StopSummaryInput{
		SessionID:     corepkg.SessionID(sess.ID),
		Reason:        fmt.Sprintf("I reached the current execution budget of %d turns before producing a final answer.", runner.config.MaxIterations),
		ExecutedTools: executedToolNames,
	})
	return e.finishWithSummary(sess, summary, out, false)
}

func (e *defaultEngine) processTurnWithReactiveCompaction(ctx context.Context, setup runPromptSetup, params turnProcessParams) (string, bool, error) {
	if e == nil || e.runner == nil {
		return "", false, fmt.Errorf("agent engine is unavailable")
	}

	runner := e.runner
	answer, finished, err := e.processTurn(ctx, params)
	if err == nil || !isPromptTooLongError(err) {
		return answer, finished, err
	}

	_, compacted, compactErr := runner.compactSession(ctx, params.Session, true, true, "reactive_prompt_too_long")
	if compactErr != nil {
		return "", false, compactErr
	}
	if !compacted {
		return "", false, err
	}
	if params.Out != nil {
		fmt.Fprintf(params.Out, "%scontext exceeded model window; compacted and retrying once%s\n", ansiDim, ansiReset)
	}

	retryMessages, buildErr := e.buildTurnMessages(params.Session, setup)
	if buildErr != nil {
		return "", false, buildErr
	}
	params.Messages = retryMessages
	return e.processTurn(ctx, params)
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
