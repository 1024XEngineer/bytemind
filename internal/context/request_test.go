package context

import (
	"testing"

	"bytemind/internal/llm"
)

func TestBuildChatRequestDropsToolsWhenModelDoesNotSupportToolUse(t *testing.T) {
	req := BuildChatRequest(ChatRequestInput{
		Model:    "custom-no-tool-model",
		Messages: []llm.Message{llm.NewUserTextMessage("hello")},
		Tools: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.FunctionDefinition{
					Name: "read_file",
				},
			},
		},
		Temperature: 0.2,
	})
	if len(req.Tools) != 0 {
		t.Fatalf("expected no tools for no-tool model, got %#v", req.Tools)
	}
}

func TestBuildChatRequestKeepsToolsWhenSupported(t *testing.T) {
	req := BuildChatRequest(ChatRequestInput{
		Model:    "gpt-5.4-mini",
		Messages: []llm.Message{llm.NewUserTextMessage("hello")},
		Tools: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.FunctionDefinition{
					Name: "read_file",
				},
			},
		},
		Temperature: 0.2,
	})
	if len(req.Tools) != 1 {
		t.Fatalf("expected tools to be preserved, got %#v", req.Tools)
	}
}

func TestBuildChatRequestAppliesMessageCapabilities(t *testing.T) {
	msg := llm.Message{
		Role: llm.RoleAssistant,
		Parts: []llm.Part{
			{
				Type: llm.PartThinking,
				Thinking: &llm.ThinkingPart{
					Value: "deep thought",
				},
			},
		},
	}

	req := BuildChatRequest(ChatRequestInput{
		Model:       "gpt-4o",
		Messages:    []llm.Message{msg},
		Temperature: 0.2,
	})
	if len(req.Messages) != 1 || len(req.Messages[0].Parts) != 1 {
		t.Fatalf("unexpected request messages: %#v", req.Messages)
	}
	if req.Messages[0].Parts[0].Type != llm.PartText {
		t.Fatalf("expected thinking to be downgraded to text, got %#v", req.Messages[0].Parts[0])
	}
}
