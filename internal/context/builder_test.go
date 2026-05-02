package context

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestBuildMessagesConcatsSystemAndConversation(t *testing.T) {
	system := llm.NewTextMessage(llm.RoleSystem, "rules")
	conversation := []llm.Message{llm.NewUserTextMessage("hello")}

	messages, err := BuildMessages(BuildRequest{
		SystemMessages:       []llm.Message{system},
		ConversationMessages: conversation,
	})
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected first message to be system, got %s", messages[0].Role)
	}
	if messages[1].Role != llm.RoleUser {
		t.Fatalf("expected second message to be user, got %s", messages[1].Role)
	}
}

func TestBuildMessagesValidatesSystemMessage(t *testing.T) {
	_, err := BuildMessages(BuildRequest{
		SystemMessages: []llm.Message{{Role: "invalid", Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestEstimateRequestTokens(t *testing.T) {
	count := EstimateRequestTokens([]llm.Message{llm.NewUserTextMessage("token test")})
	if count <= 0 {
		t.Fatalf("expected positive token estimate, got %d", count)
	}
}
