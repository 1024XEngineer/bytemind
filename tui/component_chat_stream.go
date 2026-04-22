package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m model) shouldKeepStreamingIndexOnRunFinished() bool {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return false
	}
	item := m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(item.Status))
	return status == "streaming" || status == "thinking" || status == "pending"
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := m.chatItems[m.streamingIndex].Body
		if m.chatItems[m.streamingIndex].Status == "pending" ||
			m.chatItems[m.streamingIndex].Status == "thinking" ||
			current == m.thinkingText() {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasPrefix(delta, current) {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasSuffix(current, delta) {
			// Some providers may repeat the latest chunk; ignore it.
		} else {
			m.chatItems[m.streamingIndex].Body += delta
		}
		m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   delta,
		Status: "thinking",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
}

func (m *model) applyAssistantDeltaPresentation(item *chatEntry) {
	if item == nil || item.Kind != "assistant" {
		return
	}
	if shouldRenderThinkingFromDelta(item.Body) {
		item.Title = thinkingLabel
		item.Status = "thinking"
		return
	}
	item.Title = assistantLabel
	item.Status = "streaming"
}

func (m *model) finishAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	finalContent := m.decorateFinalAnswer(content)

	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := &m.chatItems[m.streamingIndex]
		if current.Kind == "assistant" && (current.Status == "thinking" || current.Status == "pending") {
			current.Title = thinkingLabel
			current.Status = "thinking_done"
			current.Body = m.thinkingDoneText()
			m.streamingIndex = -1
			m.chatItems = append(m.chatItems, chatEntry{
				Kind:   "assistant",
				Title:  assistantLabel,
				Body:   finalContent,
				Status: "final",
			})
			return
		}

		current.Title = assistantLabel
		current.Body = finalContent
		current.Status = "final"
		m.streamingIndex = -1
		return
	}

	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && last.Title == assistantLabel && strings.TrimSpace(last.Body) == finalContent {
			last.Status = "final"
			return
		}
	}

	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   finalContent,
		Status: "final",
	})
}

func (m *model) appendChat(item chatEntry) {
	m.chatItems = append(m.chatItems, item)
}

func (m *model) finalizeAssistantTurnForTool(toolName string) {
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		item := &m.chatItems[m.streamingIndex]
		if item.Kind == "assistant" {
			if strings.TrimSpace(item.Body) == "" && (item.Status == "thinking" || item.Status == "pending") {
				item.Title = thinkingLabel
				item.Status = "thinking"
				m.streamingIndex = -1
				return
			}
			if !isMeaningfulThinking(item.Body, toolName) {
				m.removeStreamingAssistantPlaceholder()
				return
			}
			item.Title = thinkingLabel
			item.Status = "thinking"
			m.streamingIndex = -1
			return
		}
	}
}

func (m *model) removeStreamingAssistantPlaceholder() {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		m.streamingIndex = -1
		return
	}
	if m.chatItems[m.streamingIndex].Kind == "assistant" {
		m.chatItems = append(m.chatItems[:m.streamingIndex], m.chatItems[m.streamingIndex+1:]...)
	}
	m.streamingIndex = -1
}

func (m *model) appendAssistantToolFollowUp(toolName, summary, status string) {
	step := assistantToolFollowUp(toolName, summary, status)
	if step == "" {
		return
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && strings.TrimSpace(last.Body) == step {
			last.Title = thinkingLabel
			last.Status = "thinking"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   step,
		Status: "thinking",
	})
}

func (m *model) populateLatestThinkingToolStep(toolName, summary, status string) bool {
	if len(m.chatItems) == 0 {
		return false
	}
	last := &m.chatItems[len(m.chatItems)-1]
	if last.Kind != "assistant" || last.Title != thinkingLabel || last.Status != "thinking" {
		return false
	}
	if strings.TrimSpace(last.Body) != "" {
		return false
	}
	last.Body = assistantToolFollowUp(toolName, summary, status)
	return strings.TrimSpace(last.Body) != ""
}

func (m *model) finishLatestToolCall(name, body, status string) {
	title := toolEntryTitle(name)
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind != "tool" {
			continue
		}
		if m.chatItems[i].Title != title && strings.TrimSpace(name) != "" {
			continue
		}
		m.chatItems[i].Title = title
		m.chatItems[i].Body = body
		m.chatItems[i].Status = status
		return
	}
	m.appendChat(chatEntry{
		Kind:   "tool",
		Title:  title,
		Body:   body,
		Status: status,
	})
}

func (m *model) updateThinkingCard() {
	if !m.busy || m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return
	}
	item := &m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" || (item.Status != "pending" && item.Status != "thinking") {
		return
	}
	item.Title = thinkingLabel
	item.Status = "thinking"
	if strings.TrimSpace(item.Body) == "" {
		item.Body = m.thinkingText()
	}
}

func (m *model) ensureThinkingCard() {
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		item := &m.chatItems[m.streamingIndex]
		if item.Kind == "assistant" && (item.Status == "pending" || item.Status == "thinking") {
			item.Title = thinkingLabel
			item.Status = "thinking"
			if strings.TrimSpace(item.Body) == "" {
				item.Body = m.thinkingText()
			}
			return
		}
	}

	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   m.thinkingText(),
		Status: "pending",
	})
	m.streamingIndex = len(m.chatItems) - 1
}

func (m *model) failLatestAssistant(errText string) {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		errText = "Unknown provider error"
	}
	if len(m.chatItems) == 0 {
		m.appendChat(chatEntry{
			Kind:   "assistant",
			Title:  assistantLabel,
			Body:   "Request failed: " + errText,
			Status: "error",
		})
		return
	}
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind == "assistant" {
			m.chatItems[i].Body = "Request failed: " + errText
			m.chatItems[i].Status = "error"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Request failed: " + errText,
		Status: "error",
	})
}

func (m model) decorateFinalAnswer(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if m.runStartedAt.IsZero() {
		return content
	}
	if strings.Contains(content, "Completed in ") || strings.Contains(content, "Processed for ") {
		return content
	}
	churnElapsed := formatElapsedWords(m.runStartedAt, time.Now())
	return fmt.Sprintf("%s\n\nProcessed for %s", content, churnElapsed)
}

func formatElapsedWords(startedAt, now time.Time) string {
	if startedAt.IsZero() || now.Before(startedAt) {
		return "0s"
	}
	seconds := int(now.Sub(startedAt).Round(time.Second).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}
