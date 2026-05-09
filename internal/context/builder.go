package context

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

// BuildRequest defines the minimal inputs needed to assemble one model turn.
type BuildRequest struct {
	SystemMessages       []llm.Message
	ConversationMessages []llm.Message
}

// BuildMessages assembles validated request messages for one model turn.
func BuildMessages(req BuildRequest) ([]llm.Message, error) {
	messages := make([]llm.Message, 0, len(req.SystemMessages)+len(req.ConversationMessages))
	for i, message := range req.SystemMessages {
		if err := llm.ValidateMessage(message); err != nil {
			return nil, fmt.Errorf("system[%d] validation failed: %w", i, err)
		}
		messages = append(messages, message)
	}
	messages = append(messages, req.ConversationMessages...)
	return messages, nil
}

// EstimateRequestTokens approximates tokens for a complete request message set
// using a conservative ~4 chars/token heuristic.
func EstimateRequestTokens(messages []llm.Message) int {
	var total int64
	for _, msg := range messages {
		msg.Normalize()
		total += approximateTokens(msg.Text())
		for _, call := range msg.ToolCalls {
			total += approximateTokens(call.Function.Name)
			total += approximateTokens(call.Function.Arguments)
		}
	}
	return int(total)
}

func approximateTokens(text string) int64 {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	n := len([]rune(trimmed))
	if n < 1 {
		n = 1
	}
	return int64((n + 3) / 4)
}
