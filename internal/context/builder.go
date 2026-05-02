package context

import (
	"fmt"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/tokenusage"
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

// EstimateRequestTokens approximates tokens for a complete request message set.
func EstimateRequestTokens(messages []llm.Message) int {
	return int(tokenusage.ApproximateRequestTokens(messages))
}
