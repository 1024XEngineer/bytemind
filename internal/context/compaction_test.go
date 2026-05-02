package context

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestBuildPairAwareCompactedMessagesMovesBoundaryPastPairWindow(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("start"),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-a",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-b",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"a.txt"}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-a", `{"ok":true}`),
		llm.NewToolResultMessage("call-b", `{"ok":true}`),
		llm.NewAssistantTextMessage("after tools"),
		llm.NewUserTextMessage("latest ask"),
	}

	updated, fallbackUsed, err := BuildPairAwareCompactedMessages(PairAwareCompactionConfig{
		Messages:        messages,
		LatestUserIndex: 6,
		KeepPairCount:   1,
		SummaryBuilder: func(history []llm.Message) (llm.Message, error) {
			if len(history) == 0 {
				t.Fatal("expected non-empty history for summary")
			}
			return llm.NewAssistantTextMessage("Context summary:\nsummary"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fallbackUsed {
		t.Fatal("did not expect fallback for boundary adjustment case")
	}
	if len(updated) != 3 {
		t.Fatalf("expected summary + assistant tail + latest user, got %#v", updated)
	}
	if err := ValidateToolPairInvariant(updated); err != nil {
		t.Fatalf("expected pair invariant to hold, got %v", err)
	}
	if containsToolUseID(updated, "call-a") || containsToolUseID(updated, "call-b") {
		t.Fatalf("expected boundary shift to avoid half pair retention, got %#v", updated)
	}
}

func TestBuildPairAwareCompactedMessagesDropsIncompleteToolTailOnFallback(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("start"),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-1", `{"ok":true}`),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-open",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"missing.txt"}`,
				},
			}},
		},
		llm.NewUserTextMessage("latest ask"),
	}

	summaryCalls := 0
	updated, fallbackUsed, err := BuildPairAwareCompactedMessages(PairAwareCompactionConfig{
		Messages:        messages,
		LatestUserIndex: 4,
		KeepPairCount:   1,
		SummaryBuilder: func(history []llm.Message) (llm.Message, error) {
			summaryCalls++
			if len(history) == 0 {
				t.Fatal("expected non-empty history for summary")
			}
			return llm.NewAssistantTextMessage("Context summary:\nsummary"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fallbackUsed {
		t.Fatal("expected fallback when tail contains unmatched tool_use")
	}
	if summaryCalls != 2 {
		t.Fatalf("expected summary to be rebuilt once after fallback, got %d calls", summaryCalls)
	}
	if len(updated) != 2 {
		t.Fatalf("expected summary + latest user after fallback, got %#v", updated)
	}
	if containsToolUseID(updated, "call-open") {
		t.Fatalf("expected incomplete tail tool_use to be dropped, got %#v", updated)
	}
	if err := ValidateToolPairInvariant(updated); err != nil {
		t.Fatalf("expected pair invariant to hold, got %v", err)
	}
}

func TestBuildPairAwareCompactedMessagesUsesThirdAttemptWhenNeeded(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("start"),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-1", `{"ok":true}`),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"a.txt"}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-2", `{"ok":true}`),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-open",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"missing.txt"}`,
				},
			}},
		},
		llm.NewUserTextMessage("latest ask"),
	}

	summaryCalls := 0
	updated, fallbackUsed, err := BuildPairAwareCompactedMessages(PairAwareCompactionConfig{
		Messages:        messages,
		LatestUserIndex: 6,
		KeepPairCount:   2,
		SummaryBuilder: func(history []llm.Message) (llm.Message, error) {
			summaryCalls++
			if len(history) == 0 {
				t.Fatal("expected non-empty history for summary")
			}
			return llm.NewAssistantTextMessage("Context summary:\nsummary"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fallbackUsed {
		t.Fatal("expected fallback when first two attempts keep orphan tool_use")
	}
	if summaryCalls != 3 {
		t.Fatalf("expected three attempts (2 -> 1 -> 0), got %d", summaryCalls)
	}
	if containsToolUseID(updated, "call-open") {
		t.Fatalf("expected orphan tool_use to be removed after third attempt, got %#v", updated)
	}
	if err := ValidateToolPairInvariant(updated); err != nil {
		t.Fatalf("expected pair invariant to hold, got %v", err)
	}
}

func containsToolUseID(messages []llm.Message, toolUseID string) bool {
	for i := range messages {
		message := messages[i]
		message.Normalize()
		for _, part := range message.Parts {
			if part.Type != llm.PartToolUse || part.ToolUse == nil {
				continue
			}
			if part.ToolUse.ID == toolUseID {
				return true
			}
		}
	}
	return false
}
