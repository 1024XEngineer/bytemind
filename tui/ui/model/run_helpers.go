package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func isBTWCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	return fields[0] == "/btw"
}

func extractBTWText(input string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || fields[0] != "/btw" {
		return "", errors.New("usage: /btw <message>")
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), fields[0]))
	if text == "" {
		return "", errors.New("usage: /btw <message>")
	}
	return text, nil
}

func composeBTWPrompt(entries []string) string {
	cleaned := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return strings.Join([]string{
			"User sent a BTW update while you were executing an existing task.",
			"Continue the same task from the latest progress, and apply this update with high priority unless it explicitly changes the goal:",
			cleaned[0],
		}, "\n")
	}
	lines := make([]string, 0, len(cleaned)+2)
	lines = append(lines, "User sent multiple BTW updates during execution. Later items have higher priority:")
	for i, entry := range cleaned {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, entry))
	}
	lines = append(lines, "Please continue the same task with these updates and keep unfinished steps unless explicitly changed.")
	return strings.Join(lines, "\n")
}

func formatBTWUpdateScope(count int) string {
	if count <= 1 {
		return "your latest update"
	}
	return fmt.Sprintf("%d updates", count)
}

func queueBTWUpdate(queue []string, value string) ([]string, int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return queue, 0
	}
	queue = append(queue, trimmed)
	if len(queue) <= maxPendingBTW {
		return queue, 0
	}
	dropped := len(queue) - maxPendingBTW
	return append([]string(nil), queue[dropped:]...), dropped
}

func isContinueExecutionInput(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	switch normalized {
	case "continue",
		"continue execution",
		"continue plan",
		"resume",
		"resume execution",
		"继续",
		"继续执行",
		"继续做",
		"继续任务":
		return true
	default:
		return false
	}
}

func classifyRunFinish(err error, restartedByBTW bool) runFinishReason {
	if restartedByBTW {
		return runFinishReasonBTWRestart
	}
	if err == nil {
		return runFinishReasonCompleted
	}
	if errors.Is(err, context.Canceled) {
		return runFinishReasonCanceled
	}
	return runFinishReasonFailed
}
