package tui

import (
	"fmt"
	"strings"
)

func isMeaningfulThinking(body, toolName string) bool {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(raw, "`", ""))
	toolName = strings.ToLower(strings.TrimSpace(toolName))

	genericPrefixes := []string{
		"i will call ",
		"i'll call ",
		"let me call ",
		"i am going to call ",
		"i'm going to call ",
		"i will use ",
		"i'll use ",
		"let me use ",
		"i will run ",
		"let me run ",
		"i will check the relevant context first",
		"i have the tool result. let me organize the next step.",
	}
	for _, prefix := range genericPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return false
		}
	}

	if toolName != "" {
		toolIntentPhrases := []string{
			fmt.Sprintf("call %s", toolName),
			fmt.Sprintf("use %s", toolName),
			fmt.Sprintf("run %s", toolName),
		}
		for _, phrase := range toolIntentPhrases {
			if strings.Contains(normalized, phrase) && strings.Contains(normalized, "inspect") {
				return false
			}
		}
	}

	zhPrefixes := []string{
		"我会调用",
		"我会使用",
		"让我调用",
		"让我使用",
		"让我运行",
		"工具结果",
	}
	for _, prefix := range zhPrefixes {
		if strings.HasPrefix(raw, prefix) {
			return false
		}
	}

	return true
}

func shouldRenderThinkingFromDelta(body string) bool {
	text := strings.TrimSpace(body)
	if text == "" {
		return false
	}
	if !isMeaningfulThinking(text, "") {
		return false
	}
	lower := strings.ToLower(text)
	reasoningMarkers := []string{
		"i will first",
		"first,",
		"then",
		"finally",
		"approach",
		"systematically",
		"through build and test",
		"我会先",
		"先了解",
		"然后",
		"最后",
		"通过构建和测试",
		"系统性",
	}
	for _, marker := range reasoningMarkers {
		if strings.Contains(lower, marker) || strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
