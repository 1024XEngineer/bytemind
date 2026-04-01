package agent

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
)

const sessionSummaryTextLimit = 160

func summarizeSessionContext(messages []llm.Message) string {
	if len(messages) == 0 {
		return ""
	}

	lines := []string{
		fmt.Sprintf("- prior_messages: %d", len(messages)),
	}

	if text := summarizeRecentUserMessage(messages); text != "" {
		lines = append(lines, "- last_user_request: "+text)
	}
	if text := summarizeRecentAssistantMessage(messages); text != "" {
		lines = append(lines, "- last_assistant_outcome: "+text)
	}
	if names := summarizeRecentToolActivity(messages); len(names) > 0 {
		lines = append(lines, "- recent_tool_activity: "+strings.Join(names, ", "))
	}

	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func summarizeRecentUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if text := compactSummary(messages[i].Content, sessionSummaryTextLimit); text != "" {
			return text
		}
	}
	return ""
}

func summarizeRecentAssistantMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		if text := compactSummary(messages[i].Content, sessionSummaryTextLimit); text != "" {
			return text
		}
		if len(messages[i].ToolCalls) > 0 {
			return "requested tools: " + strings.Join(uniqueToolCallNames(messages[i].ToolCalls), ", ")
		}
	}
	return ""
}

func summarizeRecentToolActivity(messages []llm.Message) []string {
	names := make([]string, 0, 4)
	for _, message := range messages {
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.Function.Name) == "" {
				continue
			}
			names = append(names, call.Function.Name)
		}
	}
	return recentToolNames(names, 4)
}

func compactSummary(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
