package tui

import "strings"

func formatChatBody(item chatEntry, width int) string {
	text := strings.ReplaceAll(item.Body, "\r\n", "\n")
	if item.Kind == "user" {
		return strings.TrimRight(wrapPlainText(text, width), "\n")
	}
	if item.Kind != "assistant" {
		return strings.TrimRight(renderSemanticPlainBody(text, width), "\n")
	}
	if isHelpMarkdownText(text) {
		return strings.TrimRight(renderHelpMarkdown(text, width), "\n")
	}
	return strings.TrimRight(renderAssistantBody(text, width), "\n")
}
