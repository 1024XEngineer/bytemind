package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

type guardWebSearchTool struct{}

func (guardWebSearchTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "web_search",
			Description: "test web search",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		},
	}
}

func (guardWebSearchTool) Run(_ context.Context, raw json.RawMessage, _ *tools.ExecutionContext) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(raw, &args)
	return `{"ok":true,"query":"` + args.Query + `","results":[{"title":"Official model page","url":"https://example.com/models"}]}`, nil
}

func TestRunPromptRepairsRequiredWebLookupBeforeFinalizing(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	registry := &tools.Registry{}
	if err := registry.Register(guardWebSearchTool{}, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "gpt-5.4-mini does not exist.",
		},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-web-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "web_search",
					Arguments: `{"query":"gpt-5.4-mini official model page"}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "<turn_intent>finalize</turn_intent>Verified with web evidence.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 5,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "gpt-5.4-mini 是否存在？", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Verified with web evidence." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected repair request, web tool request, and final request; got %d", len(client.requests))
	}

	secondRequest := client.requests[1]
	lastMessage := secondRequest.Messages[len(secondRequest.Messages)-1]
	if lastMessage.Role != llm.RoleUser || !strings.Contains(lastMessage.Text(), "requires current or external web evidence") {
		t.Fatalf("expected required-web repair note in second request, got %#v", lastMessage)
	}

	if len(sess.Messages) != 4 {
		t.Fatalf("expected user + web tool call + tool result + final assistant, got %#v", sess.Messages)
	}
	if len(sess.Messages[1].ToolCalls) != 1 || sess.Messages[1].ToolCalls[0].Function.Name != "web_search" {
		t.Fatalf("expected repaired turn to execute web_search, got %#v", sess.Messages[1])
	}
	if sess.Messages[2].Role != llm.RoleUser || !strings.Contains(sess.Messages[2].Content, "Official model page") {
		t.Fatalf("expected web_search result message, got %#v", sess.Messages[2])
	}
}
