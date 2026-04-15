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

func (r *Runner) runPromptTurns(ctx context.Context, sess *session.Session, setup runPromptSetup, out io.Writer) (string, error) {
	toolSequenceTracker := runtimepkg.NewToolSequenceTracker(runtimepkg.DefaultRepeatedToolSequenceThreshold)
	executedToolNames := make([]string, 0, 16)

	for step := 0; step < r.config.MaxIterations; step++ {
		messages, err := r.messagesForStep(ctx, sess, setup, step, out)
		if err != nil {
			return "", err
		}
		answer, finished, err := r.processTurn(ctx, turnProcessParams{
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
		Reason:        fmt.Sprintf("I reached the current execution budget of %d turns before producing a final answer.", r.config.MaxIterations),
		ExecutedTools: executedToolNames,
	})
	return r.finishWithSummary(sess, summary, out, false)
}

func (r *Runner) messagesForStep(ctx context.Context, sess *session.Session, setup runPromptSetup, step int, out io.Writer) ([]llm.Message, error) {
	messages, err := r.buildTurnMessages(sess, setup)
	if err != nil {
		return nil, err
	}
	if step != 0 {
		return messages, nil
	}

	requestTokens := contextpkg.EstimateRequestTokens(messages)
	compacted, compactErr := r.maybeAutoCompactSession(ctx, sess, setup.PromptTokens, requestTokens)
	if compactErr != nil {
		return nil, compactErr
	}
	if !compacted {
		return messages, nil
	}
	if out != nil {
		fmt.Fprintf(out, "%scontext compacted to fit long-history budget%s\n", ansiDim, ansiReset)
	}
	return r.buildTurnMessages(sess, setup)
}
