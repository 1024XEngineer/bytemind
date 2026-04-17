package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"bytemind/internal/llm"
	"bytemind/internal/session"
)

func (e *defaultEngine) completeTurn(ctx context.Context, request llm.ChatRequest, out io.Writer, streamedText *bool) (llm.Message, error) {
	if e == nil || e.runner == nil {
		return llm.Message{}, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	if !runner.config.Stream {
		return runner.client.CreateMessage(ctx, request)
	}

	reply, err := runner.client.StreamMessage(ctx, request, func(delta string) {
		if delta != "" {
			emitTurnEvent(ctx, TurnEvent{
				Type: TurnEventDelta,
				Payload: map[string]any{
					"content": delta,
				},
			})
		}
		if out == nil || delta == "" {
			if delta != "" {
				runner.emit(Event{Type: EventAssistantDelta, Content: delta})
			}
			return
		}
		if !*streamedText {
			fmt.Fprintln(out)
		}
		*streamedText = true
		fmt.Fprint(out, delta)
		runner.emit(Event{Type: EventAssistantDelta, Content: delta})
	})
	if err != nil {
		return llm.Message{}, err
	}
	if strings.TrimSpace(reply.Content) != "" || len(reply.ToolCalls) > 0 {
		return reply, nil
	}

	// Some providers/models occasionally return empty streaming payloads while
	// still producing a valid non-stream completion. Retry once without stream.
	fallback, fallbackErr := runner.client.CreateMessage(ctx, request)
	if fallbackErr == nil {
		return fallback, nil
	}
	return reply, nil
}

func (e *defaultEngine) finishWithSummary(sess *session.Session, summary string, out io.Writer, streamedText bool) (string, error) {
	if e == nil || e.runner == nil {
		return "", fmt.Errorf("agent engine is unavailable")
	}

	summaryMessage := llm.NewAssistantTextMessage(summary)
	if err := e.persistAssistantReply(sess, summaryMessage); err != nil {
		return "", err
	}
	if out != nil {
		if streamedText {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, summary)
	}
	return summary, nil
}

func (r *Runner) completeTurn(ctx context.Context, request llm.ChatRequest, out io.Writer, streamedText *bool) (llm.Message, error) {
	engine := &defaultEngine{runner: r}
	return engine.completeTurn(ctx, request, out, streamedText)
}

func (r *Runner) finishWithSummary(sess *session.Session, summary string, out io.Writer, streamedText bool) (string, error) {
	engine := &defaultEngine{runner: r}
	return engine.finishWithSummary(sess, summary, out, streamedText)
}

func marshalToolResult(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"failed to encode tool result"}`
	}
	return string(data)
}
