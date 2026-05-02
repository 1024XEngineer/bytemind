package session

import (
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type MessageMetrics struct {
	RawMessageCount               int
	UserEffectiveInputCount       int
	AssistantEffectiveOutputCount int
}

func CountMessageMetrics(messages []llm.Message) MessageMetrics {
	metrics := MessageMetrics{RawMessageCount: len(messages)}
	for _, message := range messages {
		message.Normalize()
		switch message.Role {
		case llm.RoleUser:
			if isUserEffectiveInputMessage(message) {
				metrics.UserEffectiveInputCount++
			}
		case llm.RoleAssistant:
			if isAssistantEffectiveOutputMessage(message) {
				metrics.AssistantEffectiveOutputCount++
			}
		}
	}
	return metrics
}

func IsZeroMessageSession(metrics MessageMetrics) bool {
	return metrics.UserEffectiveInputCount == 0
}

func IsNoReplySession(metrics MessageMetrics) bool {
	return metrics.UserEffectiveInputCount > 0 && metrics.AssistantEffectiveOutputCount == 0
}

func isUserEffectiveInputMessage(message llm.Message) bool {
	if message.Role != llm.RoleUser {
		return false
	}
	if strings.TrimSpace(message.ToolCallID) != "" {
		return false
	}
	return hasNonEmptyTextPart(message)
}

func isAssistantEffectiveOutputMessage(message llm.Message) bool {
	if message.Role != llm.RoleAssistant {
		return false
	}
	if len(message.ToolCalls) > 0 {
		return true
	}
	return hasNonEmptyTextPart(message)
}

func hasNonEmptyTextPart(message llm.Message) bool {
	for _, part := range message.Parts {
		if part.Text != nil && strings.TrimSpace(part.Text.Value) != "" {
			return true
		}
	}
	return false
}
