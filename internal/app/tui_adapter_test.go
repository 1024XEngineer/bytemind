package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
)

type subAgentAdapterTestClient struct {
	replies  []llm.Message
	requests []llm.ChatRequest
	index    int
}

func (c *subAgentAdapterTestClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	c.requests = append(c.requests, req)
	if len(c.replies) == 0 {
		return llm.Message{}, nil
	}
	if c.index >= len(c.replies) {
		return c.replies[len(c.replies)-1], nil
	}
	reply := c.replies[c.index]
	c.index++
	return reply, nil
}

func (c *subAgentAdapterTestClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	reply, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && reply.Content != "" {
		onDelta(reply.Content)
	}
	return reply, nil
}

func TestTUIRunnerAdapterSubAgentMethods(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: builtin explorer
tools: [list_files, read_file, search_text]
---
explore files
`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := &subAgentAdapterTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "adapter delegation summary"},
		},
	}
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})
	adapter := &tuiRunnerAdapter{runner: runner}

	agents, _ := adapter.ListSubAgents()
	if len(agents) == 0 {
		t.Fatalf("expected subagents from adapter, got %#v", agents)
	}

	builtin, ok := adapter.FindBuiltinSubAgent("/explorer")
	if !ok {
		t.Fatal("expected adapter to resolve builtin explorer subagent")
	}
	if builtin.Scope != subagentspkg.ScopeBuiltin {
		t.Fatalf("expected builtin scope, got %#v", builtin)
	}

	sess := session.New(workspace)
	result, dispatchErr := adapter.DispatchSubAgent(context.Background(), sess, "build", tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "locate subagent runtime prompt block",
	})
	if dispatchErr != nil {
		t.Fatalf("expected DispatchSubAgent to succeed through adapter, got %v", dispatchErr)
	}
	if !result.OK {
		t.Fatalf("expected successful delegated subagent result through adapter, got %#v", result)
	}
}
