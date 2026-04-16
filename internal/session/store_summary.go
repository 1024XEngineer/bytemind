package session

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
)

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			messages[i].Normalize()
			textParts := make([]string, 0, len(messages[i].Parts))
			for _, part := range messages[i].Parts {
				if part.Text != nil && strings.TrimSpace(part.Text.Value) != "" {
					textParts = append(textParts, part.Text.Value)
				}
			}
			if len(textParts) > 0 {
				return strings.TrimSpace(strings.Join(textParts, " "))
			}
		}
	}
	return ""
}

func sessionTimeline(sess *Session) []llm.Message {
	if sess == nil {
		return nil
	}
	if len(sess.Conversation.Timeline) > 0 {
		return sess.Conversation.Timeline
	}
	return sess.Messages
}

func sessionTitle(sess *Session) string {
	if sess == nil {
		return ""
	}
	title := strings.TrimSpace(sess.Title)
	if title != "" {
		return title
	}
	if sess.Conversation.Meta == nil {
		return ""
	}
	raw, ok := sess.Conversation.Meta["title"]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func summarizeMessage(text string, limit int) string {
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
