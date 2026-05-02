package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
	"bytemind/tui"
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

func TestNewTUIRunnerAdapterHandlesNilRunner(t *testing.T) {
	if got := newTUIRunnerAdapter(nil); got != nil {
		t.Fatalf("expected nil adapter for nil runner, got %#v", got)
	}

	runner := newTestRunner(t, t.TempDir(), &subAgentAdapterTestClient{})
	if got := newTUIRunnerAdapter(runner); got == nil {
		t.Fatal("expected non-nil adapter for non-nil runner")
	}
}

func TestTUIRunnerAdapterNilGuardMethods(t *testing.T) {
	var adapter *tuiRunnerAdapter
	sess := session.New(t.TempDir())

	if _, err := adapter.RunPromptWithInput(context.Background(), sess, tui.RunPromptInput{}, "build", nil); err == nil {
		t.Fatal("expected runner unavailable error from RunPromptWithInput")
	}
	if _, err := adapter.ActivateSkill(sess, "review", nil); err == nil {
		t.Fatal("expected runner unavailable error from ActivateSkill")
	}
	if _, err := adapter.ClearSkill("review"); err == nil {
		t.Fatal("expected runner unavailable error from ClearSkill")
	}
	if _, _, err := adapter.CompactSession(context.Background(), sess); err == nil {
		t.Fatal("expected runner unavailable error from CompactSession")
	}
	if _, err := adapter.DispatchSubAgent(context.Background(), sess, "build", tools.DelegateSubAgentRequest{}); err == nil {
		t.Fatal("expected runner unavailable error from DispatchSubAgent")
	}

	adapter.SetObserver(nil)
	adapter.SetApprovalHandler(nil)
	adapter.UpdateProvider(config.ProviderConfig{Model: "test-model"}, nil)
	adapter.UpdateApprovalMode("on-request")

	skillsList, skillDiags := adapter.ListSkills()
	if skillsList != nil || skillDiags != nil {
		t.Fatalf("expected nil skill results for nil adapter, got %#v %#v", skillsList, skillDiags)
	}
	if _, ok := adapter.GetActiveSkill(sess); ok {
		t.Fatal("expected no active skill for nil adapter")
	}
	if err := adapter.ClearActiveSkill(sess); err != nil {
		t.Fatalf("expected nil error from ClearActiveSkill nil-guard, got %v", err)
	}
	if agents, diags := adapter.ListSubAgents(); agents != nil || diags != nil {
		t.Fatalf("expected nil subagent results for nil adapter, got %#v %#v", agents, diags)
	}
	if _, ok := adapter.FindSubAgent("review"); ok {
		t.Fatal("expected FindSubAgent false for nil adapter")
	}
	if _, ok := adapter.FindBuiltinSubAgent("/review"); ok {
		t.Fatal("expected FindBuiltinSubAgent false for nil adapter")
	}
}

func TestMapAgentEventAndType(t *testing.T) {
	source := agent.Event{
		Type:          agent.EventToolCallCompleted,
		SessionID:     "sess-1",
		UserInput:     "/review check changes",
		Content:       "done",
		ToolName:      "delegate_subagent",
		ToolArguments: `{"agent":"review"}`,
		ToolResult:    `{"ok":true}`,
		Error:         "none",
		Plan:          planpkg.State{},
		Usage:         llm.Usage{InputTokens: 12, OutputTokens: 34},
	}

	mapped := mapAgentEvent(source)
	if mapped.Type != tui.EventToolCallCompleted {
		t.Fatalf("expected mapped event type %q, got %q", tui.EventToolCallCompleted, mapped.Type)
	}
	if mapped.SessionID != "sess-1" || mapped.ToolName != "delegate_subagent" || mapped.Content != "done" {
		t.Fatalf("expected mapped event fields to be preserved, got %#v", mapped)
	}
	if mapped.Usage.InputTokens != 12 || mapped.Usage.OutputTokens != 34 {
		t.Fatalf("expected mapped usage to be preserved, got %#v", mapped.Usage)
	}

	if got := mapAgentEventType(agent.EventRunStarted); got != tui.EventRunStarted {
		t.Fatalf("expected run-started mapping, got %q", got)
	}
	const unknown = agent.EventType("custom")
	if got := mapAgentEventType(unknown); got != tui.EventType(unknown) {
		t.Fatalf("expected fallback mapping for unknown event type, got %q", got)
	}
}

func TestTUIRunnerAdapterObserverAndProviderPath(t *testing.T) {
	workspace := t.TempDir()
	runner := newTestRunner(t, workspace, &subAgentAdapterTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "adapter run"},
		},
	})
	adapter := &tuiRunnerAdapter{runner: runner}

	events := make([]tui.Event, 0, 4)
	adapter.SetObserver(func(event tui.Event) {
		events = append(events, event)
	})
	adapter.SetApprovalHandler(func(tui.ApprovalRequest) (bool, error) {
		return false, errors.New("not expected")
	})
	adapter.UpdateProvider(config.ProviderConfig{Model: "test-model"}, &subAgentAdapterTestClient{})
	adapter.UpdateApprovalMode("on-request")

	sess := session.New(workspace)
	_, runErr := adapter.RunPromptWithInput(context.Background(), sess, tui.RunPromptInput{
		UserMessage: llm.NewUserTextMessage("ping"),
		DisplayText: "ping",
	}, "build", nil)
	if runErr != nil {
		t.Fatalf("expected RunPromptWithInput through adapter to succeed, got %v", runErr)
	}
	if len(events) == 0 {
		t.Fatal("expected adapter observer to receive mapped events")
	}
}

func newTestRunner(t *testing.T, workspace string, client llm.Client) *agent.Runner {
	t.Helper()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})
}
