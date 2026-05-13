package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

var streamTurnIntentTagPattern = regexp.MustCompile(`(?is)<turn_intent>\s*[a-z_]*\s*</turn_intent>|</?turn_intent>\s*`)
var finalAnswerElapsedPattern = regexp.MustCompile(`(?is)\n+\s*(Processed for|Completed in)\s+[0-9]+[hms](?:\s+[0-9]+[hms])*\s*$`)

func stripStreamControlTags(delta string) string {
	if strings.TrimSpace(delta) == "" {
		return delta
	}
	return streamTurnIntentTagPattern.ReplaceAllString(delta, "")
}

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
	delta = stripStreamControlTags(delta)
	if delta == "" {
		return
	}
	if m.streamingIndex < 0 {
		candidate := m.suppressedAssistantDelta + delta
		if shouldRenderThinkingFromDelta(candidate) {
			m.suppressedAssistantDelta = candidate
			return
		}
		if m.suppressedAssistantDelta != "" {
			delta = m.suppressedAssistantDelta + delta
			m.suppressedAssistantDelta = ""
		}
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
	m.suppressedAssistantDelta = ""
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	finalContent := m.decorateFinalAnswer(content)
	if m.mode == modePlan {
		if planpkg.HasActiveChoice(m.plan) {
			finalContent = stripClarifyChoiceBlockFromAnswer(finalContent)
		}
		if planpkg.CanStartExecution(m.plan) {
			finalContent = stripPlanActionTailFromAnswer(finalContent)
		}
	}

	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := &m.chatItems[m.streamingIndex]
		if current.Kind == "assistant" && (current.Status == "thinking" || current.Status == "pending") {
			m.removeStreamingAssistantPlaceholder()
		} else {
			current.Title = assistantLabel
			current.Body = finalContent
			current.Status = "final"
			m.streamingIndex = -1
			return
		}
	}

	if m.finalizeTailStreamingAssistantBeforePlaceholders(finalContent) {
		return
	}

	if idx := m.latestTailOpenAssistantIndex(); idx >= 0 {
		m.chatItems[idx].Title = assistantLabel
		m.chatItems[idx].Body = finalContent
		m.chatItems[idx].Status = "final"
		m.streamingIndex = -1
		return
	}

	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && last.Title == assistantLabel && sameAssistantFinalBody(last.Body, finalContent) {
			last.Body = finalContent
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

func (m *model) finalizeTailStreamingAssistantBeforePlaceholders(finalContent string) bool {
	idx := len(m.chatItems) - 1
	for idx >= 0 && isAssistantTransientPlaceholder(m.chatItems[idx]) {
		idx--
	}
	if idx < 0 || idx == len(m.chatItems)-1 || !isAssistantStreamingAnswer(m.chatItems[idx]) {
		return false
	}
	m.chatItems = m.chatItems[:idx+1]
	m.chatItems[idx].Title = assistantLabel
	m.chatItems[idx].Body = finalContent
	m.chatItems[idx].Status = "final"
	m.streamingIndex = -1
	return true
}

func isAssistantStreamingAnswer(item chatEntry) bool {
	if item.Kind != "assistant" {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(item.Status)) {
	case "streaming", "settling":
		return true
	default:
		return false
	}
}

func isAssistantTransientPlaceholder(item chatEntry) bool {
	if item.Kind != "assistant" {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(item.Status)) {
	case "thinking", "pending":
		return true
	default:
		return false
	}
}

func (m model) latestTailOpenAssistantIndex() int {
	if len(m.chatItems) == 0 {
		return -1
	}
	index := len(m.chatItems) - 1
	item := m.chatItems[index]
	if item.Kind != "assistant" {
		return -1
	}
	switch strings.TrimSpace(strings.ToLower(item.Status)) {
	case "pending", "thinking", "streaming", "settling":
		return index
	default:
		return -1
	}
}

func sameAssistantFinalBody(a, b string) bool {
	return normalizeAssistantFinalBody(a) == normalizeAssistantFinalBody(b)
}

func normalizeAssistantFinalBody(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return strings.TrimSpace(finalAnswerElapsedPattern.ReplaceAllString(content, ""))
}

func (m *model) appendChat(item chatEntry) {
	m.chatItems = append(m.chatItems, item)
}

func (m *model) removeThinkingCard() {
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		item := m.chatItems[i]
		if item.Kind == "assistant" && (item.Status == "thinking" || item.Status == "pending") {
			m.chatItems = append(m.chatItems[:i], m.chatItems[i+1:]...)
			return
		}
	}
}

func (m *model) finalizeAssistantTurnForTool(_ string) {
	m.suppressedAssistantDelta = ""
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		item := &m.chatItems[m.streamingIndex]
		if item.Kind == "assistant" {
			if item.Status == "thinking" || item.Status == "pending" || item.Status == "streaming" {
				m.removeStreamingAssistantPlaceholder()
				return
			}
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

// finishToolCall finds the tool chatEntry by ToolCallID (precise match)
// and updates its Body, Status, CompactBody, and DetailLines.
// If no matching entry is found, appends as a new entry.
func (m *model) finishToolCall(toolCallID, name, body, status string, compactBody string, detailLines []string) {
	if toolCallID != "" {
		for i := len(m.chatItems) - 1; i >= 0; i-- {
			if m.chatItems[i].Kind == "tool" && m.chatItems[i].ToolCallID == toolCallID {
				m.chatItems[i].Body = body
				m.chatItems[i].Status = status
				m.chatItems[i].CompactBody = compactBody
				m.chatItems[i].DetailLines = detailLines
				return
			}
		}
	}
	title := toolEntryTitle(name)
	m.appendChat(chatEntry{
		Kind:        "tool",
		Title:       title,
		Body:        body,
		Status:      status,
		CompactBody: compactBody,
		DetailLines: detailLines,
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
	} else if m.reasoningProgressActive {
		item.Body = "receiving hidden reasoning..."
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

func (m *model) applyReasoningProgress(_ int, active bool) {
	m.reasoningProgressActive = active
	m.ensureThinkingCard()
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return
	}
	if item := &m.chatItems[m.streamingIndex]; item.Kind == "assistant" && (item.Status == "pending" || item.Status == "thinking") {
		item.Title = thinkingLabel
		item.Status = "thinking"
		if active {
			item.Body = "receiving hidden reasoning..."
		} else {
			item.Body = m.thinkingText()
		}
	}
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
			m.chatItems[i].Title = assistantLabel
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

func (m *model) failRunningToolCalls() {
	for i := range m.chatItems {
		if m.chatItems[i].Kind == "tool" && (m.chatItems[i].Status == "running" || m.chatItems[i].Status == "queued") {
			m.chatItems[i].Status = "error"
		}
	}
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
