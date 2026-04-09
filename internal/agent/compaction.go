package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"bytemind/internal/llm"
	"bytemind/internal/session"
)

const (
	autoCompactThresholdRatio    = 95
	maxCompactionSummaryRunes    = 4000
	maxCompactionTranscriptRunes = 48000
	maxCompactionMessageRunes    = 1200
)

const compactionSystemPrompt = `You are a conversation compaction assistant for a coding agent.
Create a concise continuation summary for the next model call.
Preserve only durable, execution-relevant facts:
- user goal and scope
- decisions and constraints
- completed work and remaining tasks
- key file paths / commands / errors
- unresolved questions

Rules:
- Do not invent facts.
- Remove repetitive chatter and low-signal tool noise.
- Keep the summary compact and actionable.`

func (r *Runner) compactThresholdTokens() int {
	quota := r.config.TokenQuota
	if quota < 1 {
		quota = 5000
	}
	threshold := quota * autoCompactThresholdRatio / 100
	if threshold < 1 {
		return 1
	}
	return threshold
}

func (r *Runner) maybeAutoCompactSession(ctx context.Context, sess *session.Session, promptTokens, requestTokens int) (bool, error) {
	threshold := r.compactThresholdTokens()
	if promptTokens >= threshold {
		return false, fmt.Errorf("prompt too long (%d estimated tokens) for current context budget (%d). Try a shorter prompt or split it", promptTokens, threshold)
	}
	if requestTokens <= threshold {
		return false, nil
	}
	_, changed, err := r.compactSession(ctx, sess, true, "auto")
	if err != nil {
		return false, err
	}
	return changed, nil
}

func (r *Runner) CompactSession(ctx context.Context, sess *session.Session) (string, bool, error) {
	return r.compactSession(ctx, sess, false, "manual")
}

func (r *Runner) compactSession(ctx context.Context, sess *session.Session, keepLatestUser bool, reason string) (string, bool, error) {
	if sess == nil {
		return "", false, fmt.Errorf("session is required")
	}
	if r.client == nil {
		return "", false, fmt.Errorf("llm client is unavailable")
	}
	if len(sess.Messages) < 2 {
		return "", false, nil
	}

	messages := cloneMessages(sess.Messages)
	latestUserIndex := -1
	if keepLatestUser {
		for i := len(messages) - 1; i >= 0; i-- {
			if isHumanUserMessage(messages[i]) {
				latestUserIndex = i
				break
			}
		}
	}

	history := make([]llm.Message, 0, len(messages))
	var preserved llm.Message
	for i, message := range messages {
		if i == latestUserIndex {
			preserved = message
			continue
		}
		history = append(history, message)
	}
	if len(history) == 0 {
		return "", false, nil
	}

	summary, err := r.requestCompactionSummary(ctx, history)
	if err != nil {
		return "", false, err
	}
	summary = strings.TrimSpace(truncateRunes(summary, maxCompactionSummaryRunes))
	if summary == "" {
		summary = fallbackCompactionSummary(history)
	}
	if summary == "" {
		return "", false, fmt.Errorf("compaction returned empty summary")
	}

	summaryMessage := llm.NewAssistantTextMessage("Context summary:\n" + summary)
	summaryMessage.Meta = llm.MessageMeta{
		"compaction": map[string]any{
			"reason":               strings.TrimSpace(reason),
			"created_at":           time.Now().UTC().Format(time.RFC3339),
			"message_count_before": len(messages),
		},
	}

	updated := []llm.Message{summaryMessage}
	if latestUserIndex >= 0 {
		preserved.Normalize()
		updated = append(updated, preserved)
	}
	for i := range updated {
		if err := llm.ValidateMessage(updated[i]); err != nil {
			return "", false, fmt.Errorf("invalid compacted message at %d: %w", i, err)
		}
	}

	sess.Messages = updated
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return "", false, err
		}
	}
	return summary, true, nil
}

func (r *Runner) requestCompactionSummary(ctx context.Context, history []llm.Message) (string, error) {
	transcript := buildCompactionTranscript(history, maxCompactionTranscriptRunes)
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("compaction transcript is empty")
	}

	firstGoal := firstUserGoal(history)
	if strings.TrimSpace(firstGoal) == "" {
		firstGoal = "(unknown)"
	}

	prompt := strings.TrimSpace(strings.Join([]string{
		"First user goal:",
		firstGoal,
		"",
		"Conversation transcript:",
		transcript,
		"",
		"Return only the compact continuation summary.",
	}, "\n"))

	request := llm.ChatRequest{
		Model:       r.config.Provider.Model,
		Temperature: 0,
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, compactionSystemPrompt),
			llm.NewUserTextMessage(prompt),
		},
	}

	reply, err := r.client.CreateMessage(ctx, request)
	if err != nil {
		return "", err
	}
	reply.Normalize()
	return strings.TrimSpace(reply.Text()), nil
}

func fallbackCompactionSummary(history []llm.Message) string {
	if len(history) == 0 {
		return ""
	}

	goal := strings.TrimSpace(firstUserGoal(history))
	if goal == "" {
		goal = "(unknown)"
	}

	const recentCount = 6
	start := 0
	if len(history) > recentCount {
		start = len(history) - recentCount
	}

	lines := []string{
		"Compaction fallback summary (model returned empty summary).",
		"User goal: " + truncateRunes(goal, 320),
		"Recent context:",
	}

	for i := start; i < len(history); i++ {
		entry := strings.TrimSpace(formatCompactionMessage(i+1, history[i]))
		if entry == "" {
			continue
		}
		lines = append(lines, "- "+truncateRunes(entry, 420))
	}

	return strings.TrimSpace(truncateRunes(strings.Join(lines, "\n"), maxCompactionSummaryRunes))
}

func buildCompactionTranscript(messages []llm.Message, limit int) string {
	if len(messages) == 0 || limit <= 0 {
		return ""
	}

	lines := make([]string, 0, len(messages))
	used := 0
	for i := range messages {
		line := formatCompactionMessage(i+1, messages[i])
		if strings.TrimSpace(line) == "" {
			continue
		}
		lineRunes := utf8.RuneCountInString(line)
		separatorRunes := 0
		if len(lines) > 0 {
			separatorRunes = 2
		}
		if used+separatorRunes+lineRunes > limit {
			remaining := limit - used - separatorRunes
			if remaining > 32 {
				lines = append(lines, truncateRunes(line, remaining))
			}
			lines = append(lines, "[...older details omitted...]")
			break
		}
		lines = append(lines, line)
		used += separatorRunes + lineRunes
	}

	return strings.Join(lines, "\n\n")
}

func formatCompactionMessage(index int, message llm.Message) string {
	message.Normalize()
	snippets := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case llm.PartText:
			if part.Text != nil {
				snippets = append(snippets, compactForCompaction(part.Text.Value))
			}
		case llm.PartThinking:
			if part.Thinking != nil {
				snippets = append(snippets, "thinking: "+compactForCompaction(part.Thinking.Value))
			}
		case llm.PartToolUse:
			if part.ToolUse != nil {
				name := strings.TrimSpace(part.ToolUse.Name)
				args := compactForCompaction(part.ToolUse.Arguments)
				snippets = append(snippets, fmt.Sprintf("tool_use %s %s", name, args))
			}
		case llm.PartToolResult:
			if part.ToolResult != nil {
				snippets = append(snippets, "tool_result "+compactForCompaction(part.ToolResult.Content))
			}
		case llm.PartImageRef:
			if part.Image != nil {
				snippets = append(snippets, "image_ref "+strings.TrimSpace(string(part.Image.AssetID)))
			}
		}
	}
	if len(snippets) == 0 {
		return ""
	}

	text := truncateRunes(strings.Join(snippets, " | "), maxCompactionMessageRunes)
	return fmt.Sprintf("%03d %s: %s", index, strings.TrimSpace(string(message.Role)), text)
}

func compactForCompaction(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return truncateRunes(value, maxCompactionMessageRunes)
}

func firstUserGoal(messages []llm.Message) string {
	for i := range messages {
		if !isHumanUserMessage(messages[i]) {
			continue
		}
		text := strings.TrimSpace(messages[i].Text())
		if text != "" {
			return text
		}
	}
	return ""
}

func isHumanUserMessage(message llm.Message) bool {
	message.Normalize()
	if message.Role != llm.RoleUser {
		return false
	}
	hasHumanPart := false
	for _, part := range message.Parts {
		switch part.Type {
		case llm.PartText, llm.PartImageRef:
			hasHumanPart = true
		}
	}
	return hasHumanPart
}

func cloneMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]llm.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
