package context

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestBuildTurnMessagesIncludesSystemAndConversation(t *testing.T) {
	messages, err := BuildTurnMessages(TurnMessagesRequest{
		SystemPrompt:         "system rules",
		ConversationMessages: []llm.Message{llm.NewUserTextMessage("hello")},
	})
	if err != nil {
		t.Fatalf("BuildTurnMessages failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected first message role system, got %s", messages[0].Role)
	}
	if messages[1].Role != llm.RoleUser {
		t.Fatalf("expected second message role user, got %s", messages[1].Role)
	}
}

func TestBuildTurnMessagesIncludesWebLookupInstructionWhenProvided(t *testing.T) {
	messages, err := BuildTurnMessages(TurnMessagesRequest{
		SystemPrompt:         "system rules",
		WebLookupInstruction: "must browse before answer",
		ConversationMessages: []llm.Message{llm.NewUserTextMessage("latest news")},
	})
	if err != nil {
		t.Fatalf("BuildTurnMessages failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[1].Role != llm.RoleSystem {
		t.Fatalf("expected second message role system, got %s", messages[1].Role)
	}
}

func TestBuildTurnMessagesValidatesSystemPrompt(t *testing.T) {
	_, err := BuildTurnMessages(TurnMessagesRequest{
		SystemPrompt: "   ",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
