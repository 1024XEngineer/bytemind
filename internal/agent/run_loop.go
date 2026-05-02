package agent

import (
	"context"
	"io"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

func (r *Runner) runPromptTurns(ctx context.Context, sess *session.Session, setup runPromptSetup, out io.Writer) (string, error) {
	engine := &defaultEngine{runner: r}
	return engine.runPromptTurns(ctx, sess, setup, out)
}

func (r *Runner) processTurnWithReactiveCompaction(ctx context.Context, setup runPromptSetup, params turnProcessParams) (string, bool, error) {
	engine := &defaultEngine{runner: r}
	return engine.processTurnWithReactiveCompaction(ctx, setup, params)
}

func (r *Runner) messagesForStep(ctx context.Context, sess *session.Session, setup runPromptSetup, step int, out io.Writer) ([]llm.Message, error) {
	engine := &defaultEngine{runner: r}
	return engine.messagesForStep(ctx, sess, setup, step, out)
}
