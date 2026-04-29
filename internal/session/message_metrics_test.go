package session

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestCountMessageMetricsCountsRawUserAndAssistantOutputs(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("write tests"),
		llm.NewToolResultMessage("call-1", `{"ok":true}`),
		llm.NewAssistantTextMessage("working on it"),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "run_shell",
					Arguments: `{"command":"go test ./..."}`,
				},
			}},
		},
	}

	metrics := CountMessageMetrics(messages)
	if metrics.RawMessageCount != 4 {
		t.Fatalf("expected raw count 4, got %+v", metrics)
	}
	if metrics.UserEffectiveInputCount != 1 {
		t.Fatalf("expected one effective user input, got %+v", metrics)
	}
	if metrics.AssistantEffectiveOutputCount != 2 {
		t.Fatalf("expected two effective assistant outputs, got %+v", metrics)
	}
	if IsZeroMessageSession(metrics) {
		t.Fatalf("expected non-zero session, got %+v", metrics)
	}
	if IsNoReplySession(metrics) {
		t.Fatalf("expected replied session, got %+v", metrics)
	}
}

func TestCountMessageMetricsExcludesLegacyToolResultPayload(t *testing.T) {
	legacyToolResult := llm.Message{
		Role:       llm.RoleUser,
		ToolCallID: "tool-legacy",
		Content:    `{"ok":false}`,
	}
	metrics := CountMessageMetrics([]llm.Message{legacyToolResult})
	if metrics.UserEffectiveInputCount != 0 {
		t.Fatalf("expected legacy tool_result payload to be excluded from user input, got %+v", metrics)
	}
	if !IsZeroMessageSession(metrics) {
		t.Fatalf("expected zero-message session classification, got %+v", metrics)
	}
}

func TestCountMessageMetricsCountsAssistantToolRetryAttempts(t *testing.T) {
	messages := []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-failed",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "search_text",
					Arguments: `{"pattern":"TODO"}`,
				},
			}},
		},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-retry",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "search_text",
					Arguments: `{"pattern":"TODO"}`,
				},
			}},
		},
	}
	metrics := CountMessageMetrics(messages)
	if metrics.AssistantEffectiveOutputCount != 2 {
		t.Fatalf("expected retry attempts to count as effective assistant outputs, got %+v", metrics)
	}
}

func TestCountMessageMetricsClassifiesNoReplySession(t *testing.T) {
	metrics := CountMessageMetrics([]llm.Message{
		llm.NewUserTextMessage("waiting for reply"),
	})
	if IsZeroMessageSession(metrics) {
		t.Fatalf("expected no-reply session not to be zero-message, got %+v", metrics)
	}
	if !IsNoReplySession(metrics) {
		t.Fatalf("expected no-reply classification, got %+v", metrics)
	}
}

func TestCountMessageMetricsIgnoresNonTextAndNonAssistantRoles(t *testing.T) {
	messages := []llm.Message{
		{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Type:  llm.PartImageRef,
				Image: &llm.ImagePartRef{AssetID: "asset-1"},
			}},
		},
		{
			Role: llm.RoleSystem,
			Parts: []llm.Part{{
				Type: llm.PartText,
				Text: &llm.TextPart{Value: "ignored system text"},
			}},
		},
		{
			Role: llm.RoleAssistant,
			Parts: []llm.Part{{
				Type: llm.PartText,
				Text: &llm.TextPart{Value: "   "},
			}},
		},
	}

	metrics := CountMessageMetrics(messages)
	if metrics.UserEffectiveInputCount != 0 {
		t.Fatalf("expected non-text user input to be ignored, got %+v", metrics)
	}
	if metrics.AssistantEffectiveOutputCount != 0 {
		t.Fatalf("expected empty assistant text to be ignored, got %+v", metrics)
	}
}

func TestMessageMetricHelpersRejectWrongRoles(t *testing.T) {
	if isUserEffectiveInputMessage(llm.NewAssistantTextMessage("assistant")) {
		t.Fatal("expected non-user message to be rejected by user-effective-input helper")
	}
	if isAssistantEffectiveOutputMessage(llm.NewUserTextMessage("user")) {
		t.Fatal("expected non-assistant message to be rejected by assistant-effective-output helper")
	}
}
