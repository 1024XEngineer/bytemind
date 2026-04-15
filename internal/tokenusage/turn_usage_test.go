package tokenusage

import (
	"testing"

	"bytemind/internal/llm"
)

func TestResolveTurnUsageUsesProviderUsageWhenAvailable(t *testing.T) {
	reply := &llm.Message{
		Role: llm.RoleAssistant,
		Usage: &llm.Usage{
			InputTokens:   10,
			OutputTokens:  5,
			ContextTokens: 3,
			TotalTokens:   0,
		},
	}
	usage := ResolveTurnUsage(llm.ChatRequest{}, reply)
	if usage.InputTokens != 10 || usage.OutputTokens != 5 || usage.ContextTokens != 3 || usage.TotalTokens != 18 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestResolveTurnUsageFallsBackToApproximation(t *testing.T) {
	request := llm.ChatRequest{
		Messages: []llm.Message{
			llm.NewUserTextMessage("hello world"),
		},
	}
	reply := &llm.Message{
		Role:    llm.RoleAssistant,
		Content: "done",
		ToolCalls: []llm.ToolCall{
			{
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"a.go"}`,
				},
			},
		},
	}

	usage := ResolveTurnUsage(request, reply)
	if usage.InputTokens <= 0 {
		t.Fatalf("expected positive input approximation, got %#v", usage)
	}
	if usage.OutputTokens <= 0 {
		t.Fatalf("expected positive output approximation, got %#v", usage)
	}
	if usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		t.Fatalf("unexpected total approximation: %#v", usage)
	}
}
