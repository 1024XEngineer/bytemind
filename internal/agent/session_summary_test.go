package agent

import (
	"strings"
	"testing"

	"bytemind/internal/llm"
)

func TestSummarizeSessionContextEmptyWhenNoPriorMessages(t *testing.T) {
	if got := summarizeSessionContext(nil); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
}

func TestSummarizeSessionContextIncludesRecentUserAssistantAndTools(t *testing.T) {
	got := summarizeSessionContext([]llm.Message{
		{Role: "user", Content: "Inspect prompt architecture and suggest improvements."},
		{
			Role:    "assistant",
			Content: "I found the mode wiring is still fixed to build and the prompt blocks are optional.",
			ToolCalls: []llm.ToolCall{
				{Function: llm.ToolFunctionCall{Name: "read_file"}},
				{Function: llm.ToolFunctionCall{Name: "update_plan"}},
			},
		},
	})

	for _, want := range []string{
		"- prior_messages: 2",
		"- last_user_request: Inspect prompt architecture and suggest improvements.",
		"- last_assistant_outcome: I found the mode wiring is still fixed to build and the prompt blocks are optional.",
		"- recent_tool_activity: read_file, update_plan",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in summary, got %q", want, got)
		}
	}
}

func TestSummarizeSessionContextFallsBackToToolSummary(t *testing.T) {
	got := summarizeSessionContext([]llm.Message{
		{Role: "user", Content: "Continue."},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{Function: llm.ToolFunctionCall{Name: "search_text"}},
				{Function: llm.ToolFunctionCall{Name: "read_file"}},
			},
		},
	})

	if !strings.Contains(got, "- last_assistant_outcome: requested tools: search_text, read_file") {
		t.Fatalf("expected tool fallback in summary, got %q", got)
	}
}
