package context

import (
	"strings"

	"bytemind/internal/llm"
)

// TurnMessagesRequest defines one model turn message assembly contract.
type TurnMessagesRequest struct {
	SystemPrompt         string
	WebLookupInstruction string
	ConversationMessages []llm.Message
}

// BuildTurnMessages assembles system + optional web-lookup + conversation messages.
func BuildTurnMessages(req TurnMessagesRequest) ([]llm.Message, error) {
	systemMessages := make([]llm.Message, 0, 2)

	systemMessage := llm.NewTextMessage(llm.RoleSystem, req.SystemPrompt)
	if err := llm.ValidateMessage(systemMessage); err != nil {
		return nil, err
	}
	systemMessages = append(systemMessages, systemMessage)

	if strings.TrimSpace(req.WebLookupInstruction) != "" {
		webLookupMessage := llm.NewTextMessage(llm.RoleSystem, req.WebLookupInstruction)
		if err := llm.ValidateMessage(webLookupMessage); err != nil {
			return nil, err
		}
		systemMessages = append(systemMessages, webLookupMessage)
	}

	return BuildMessages(BuildRequest{
		SystemMessages:       systemMessages,
		ConversationMessages: req.ConversationMessages,
	})
}
