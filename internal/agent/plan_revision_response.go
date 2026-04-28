package agent

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
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

func shouldCondensePlanRevisionAnswer(runMode planpkg.AgentMode, state planpkg.State, messages []llm.Message) bool {
	if runMode != planpkg.ModePlan {
		return false
	}
	state = planpkg.NormalizeState(state)
	if !planpkg.CanStartExecution(state) {
		return false
	}
	index, latestUser := latestHumanUserMessage(messages)
	if index < 0 || !looksLikePlanRevisionInput(latestUser) {
		return false
	}
	return hasToolActivitySinceIndex(messages, index)
}

func condensePlanRevisionAnswer(answer string, latestUser string) string {
	trimmed := strings.TrimSpace(answer)
	if isShortPlanRevisionAcknowledgement(trimmed) {
		return trimmed
	}
	return localizedPlanRevisionAcknowledgement(latestUser)
}

func localizedPlanRevisionAcknowledgement(source string) string {
	if looksMostlyChinese(source) {
		return "已按你的反馈更新计划，下面是修订后的完整版本。"
	}
	return "Updated the plan with your feedback. The revised full plan is below."
}

func isShortPlanRevisionAcknowledgement(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.Contains(text, "\n") || utf8.RuneCountInString(text) > 80 {
		return false
	}
	normalized := strings.ToLower(text)
	if strings.Count(text, "，")+strings.Count(text, ",")+strings.Count(text, ";")+strings.Count(text, "；") > 1 {
		return false
	}
	if containsAnyToken(normalized,
		"建议",
		"比如",
		"页面",
		"功能",
		"布局",
		"交互",
		"细节",
		"设计",
		"feature",
		"features",
		"layout",
		"interaction",
		"spec",
		"specification",
		"design",
		"suggest",
		"recommend",
		"proposal",
	) {
		return false
	}
	return !strings.Contains(normalized, "- ") &&
		!strings.Contains(normalized, "##") &&
		!strings.Contains(normalized, "###") &&
		!strings.Contains(normalized, "1.") &&
		!strings.Contains(normalized, "2.")
}

func looksMostlyChinese(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	total := 0
	han := 0
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		total++
		if unicode.Is(unicode.Han, r) {
			han++
		}
	}
	return total > 0 && han*2 >= total
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
