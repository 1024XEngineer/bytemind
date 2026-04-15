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

func (r *Runner) finishWithSummary(sess *session.Session, summary string, out io.Writer, streamedText bool) (string, error) {
	summaryMessage := llm.NewAssistantTextMessage(summary)
	if err := r.persistAssistantReply(sess, summaryMessage); err != nil {
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

func marshalToolResult(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"failed to encode tool result"}`
	}
	return string(data)
}
