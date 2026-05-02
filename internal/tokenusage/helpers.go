package tokenusage

import (
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

// ApproximateTokens uses a conservative fallback estimator (~4 chars/token).
func ApproximateTokens(text string) int64 {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	n := len([]rune(trimmed))
	return int64(max(1, (n+3)/4))
}

// ApproximateRequestTokens estimates the current request token size.
func ApproximateRequestTokens(messages []llm.Message) int64 {
	var total int64
	for _, msg := range messages {
		msg.Normalize()
		total += ApproximateTokens(msg.Text())
		for _, call := range msg.ToolCalls {
			total += ApproximateTokens(call.Function.Name)
			total += ApproximateTokens(call.Function.Arguments)
		}
	}
	return total
}
