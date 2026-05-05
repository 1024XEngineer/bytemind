package agent

import (
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func latestHumanUserMessageText(messages []llm.Message) string {
	_, text := latestHumanUserMessage(messages)
	return text
}

func latestHumanUserMessage(messages []llm.Message) (int, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != llm.RoleUser || isToolResultMessage(msg) {
			continue
		}
		text := strings.TrimSpace(msg.Text())
		if text != "" {
			return i, text
		}
	}
	return -1, ""
}

func hasToolActivitySinceLatestHumanUser(messages []llm.Message) bool {
	index, _ := latestHumanUserMessage(messages)
	if index < 0 {
		return false
	}
	return hasToolActivitySinceIndex(messages, index)
}

func hasToolActivitySinceIndex(messages []llm.Message, index int) bool {
	for i := index + 1; i < len(messages); i++ {
		msg := messages[i]
		if len(msg.ToolCalls) > 0 || isToolResultMessage(msg) {
			return true
		}
	}
	return false
}

func isToolResultMessage(msg llm.Message) bool {
	if msg.Role != llm.RoleUser {
		return false
	}
	if strings.TrimSpace(msg.ToolCallID) != "" {
		return true
	}
	for _, part := range msg.Parts {
		if part.ToolResult != nil {
			return true
		}
	}
	return false
}
