package app

import (
	"context"
	"io"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tui"
)

type adapterTestClient struct {
	reply    llm.Message
	requests []llm.ChatRequest
}

func (c *adapterTestClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	c.requests = append(c.requests, req)
	if c.reply.Role == "" {
		return llm.NewAssistantTextMessage("adapter reply"), nil
	}
	return c.reply, nil
}

func (c *adapterTestClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	reply, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil {
		onDelta(reply.Text())
	}
	return reply, nil
}

func newAdapterTestRunner(t *testing.T, client llm.Client) *agent.Runner {
	t.Helper()
	return agent.NewRunner(agent.Options{
		Workspace: t.TempDir(),
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "model-a"},
			Stream:        false,
			MaxIterations: 2,
		},
		Client: client,
	})
}

func TestNewTUIRunnerAdapter(t *testing.T) {
	if got := newTUIRunnerAdapter(nil); got != nil {
		t.Fatalf("expected nil adapter when runner is nil")
	}

	runner := newAdapterTestRunner(t, &adapterTestClient{})
	if got := newTUIRunnerAdapter(runner); got == nil {
		t.Fatalf("expected non-nil adapter when runner exists")
	}
}

func TestTUIRunnerAdapterNilGuards(t *testing.T) {
	var adapter *tuiRunnerAdapter
	sess := session.New(t.TempDir())

	if _, err := adapter.RunPromptWithInput(context.Background(), sess, tui.RunPromptInput{}, "build", io.Discard); err == nil {
		t.Fatalf("expected run to fail when adapter is nil")
	}
	adapter.SetObserver(nil)
	adapter.SetApprovalHandler(nil)
	adapter.UpdateProvider(config.ProviderConfig{Model: "unused"}, nil)

	skillsList, diagnostics := adapter.ListSkills()
	if skillsList != nil || diagnostics != nil {
		t.Fatalf("expected nil skills and diagnostics for nil adapter")
	}

	if _, ok := adapter.GetActiveSkill(sess); ok {
		t.Fatalf("expected no active skill for nil adapter")
	}
	if _, err := adapter.ActivateSkill(sess, "missing", nil); err == nil {
		t.Fatalf("expected activate skill to fail for nil adapter")
	}
	if err := adapter.ClearActiveSkill(sess); err != nil {
		t.Fatalf("expected clear active skill to no-op for nil adapter, got %v", err)
	}
	if _, err := adapter.ClearSkill("missing"); err == nil {
		t.Fatalf("expected clear skill to fail for nil adapter")
	}
	if _, _, err := adapter.CompactSession(context.Background(), sess); err == nil {
		t.Fatalf("expected compact session to fail for nil adapter")
	}
}

func TestTUIRunnerAdapterRunPromptAndUpdateProvider(t *testing.T) {
	client := &adapterTestClient{reply: llm.NewAssistantTextMessage("adapter ok")}
	runner := newAdapterTestRunner(t, client)
	adapterAny := newTUIRunnerAdapter(runner)
	adapter, ok := adapterAny.(*tuiRunnerAdapter)
	if !ok {
		t.Fatalf("expected concrete adapter type")
	}

	events := make([]tui.Event, 0, 8)
	adapter.SetObserver(func(event tui.Event) {
		events = append(events, event)
	})

	sess := session.New(t.TempDir())
	answer, err := adapter.RunPromptWithInput(context.Background(), sess, tui.RunPromptInput{
		UserMessage: llm.NewUserTextMessage("hello"),
		DisplayText: "hello",
	}, "build", io.Discard)
	if err != nil {
		t.Fatalf("expected run prompt to succeed, got %v", err)
	}
	if answer == "" {
		t.Fatalf("expected non-empty assistant answer")
	}
	if len(events) == 0 {
		t.Fatalf("expected observer to receive mapped events")
	}
	if !hasEventType(events, tui.EventRunStarted) || !hasEventType(events, tui.EventRunFinished) {
		t.Fatalf("expected run_started and run_finished events, got %+v", events)
	}
	if len(client.requests) == 0 || client.requests[0].Model != "model-a" {
		t.Fatalf("expected first request to use model-a, got %+v", client.requests)
	}

	adapter.UpdateProvider(config.ProviderConfig{Model: "model-b"}, nil)
	sess2 := session.New(t.TempDir())
	if _, err := adapter.RunPromptWithInput(context.Background(), sess2, tui.RunPromptInput{
		UserMessage: llm.NewUserTextMessage("again"),
		DisplayText: "again",
	}, "build", io.Discard); err != nil {
		t.Fatalf("expected second run prompt to succeed, got %v", err)
	}
	if client.requests[len(client.requests)-1].Model != "model-b" {
		t.Fatalf("expected updated model model-b, got %q", client.requests[len(client.requests)-1].Model)
	}
}

func TestTUIRunnerAdapterSkillAndCompactionMethods(t *testing.T) {
	client := &adapterTestClient{}
	runner := newAdapterTestRunner(t, client)
	adapterAny := newTUIRunnerAdapter(runner)
	adapter, ok := adapterAny.(*tuiRunnerAdapter)
	if !ok {
		t.Fatalf("expected concrete adapter type")
	}

	sess := session.New(t.TempDir())
	_, _ = adapter.ListSkills()
	if _, ok := adapter.GetActiveSkill(sess); ok {
		t.Fatalf("expected no active skill for a new session")
	}
	if err := adapter.ClearActiveSkill(sess); err != nil {
		t.Fatalf("expected clear active skill to succeed, got %v", err)
	}
	if _, err := adapter.ActivateSkill(sess, "missing", nil); err == nil {
		t.Fatalf("expected activating unknown skill to fail")
	}
	if _, err := adapter.ClearSkill("missing"); err == nil {
		t.Fatalf("expected clearing unknown skill to fail")
	}
	summary, changed, err := adapter.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatalf("expected compact session to succeed on short history, got %v", err)
	}
	if changed || summary != "" {
		t.Fatalf("expected no compaction on empty session, got summary=%q changed=%v", summary, changed)
	}
}

func TestMapAgentEventTypeAndEvent(t *testing.T) {
	cases := []struct {
		name string
		in   agent.EventType
		want tui.EventType
	}{
		{name: "run started", in: agent.EventRunStarted, want: tui.EventRunStarted},
		{name: "assistant delta", in: agent.EventAssistantDelta, want: tui.EventAssistantDelta},
		{name: "assistant message", in: agent.EventAssistantMessage, want: tui.EventAssistantMessage},
		{name: "tool started", in: agent.EventToolCallStarted, want: tui.EventToolCallStarted},
		{name: "tool completed", in: agent.EventToolCallCompleted, want: tui.EventToolCallCompleted},
		{name: "plan", in: agent.EventPlanUpdated, want: tui.EventPlanUpdated},
		{name: "usage", in: agent.EventUsageUpdated, want: tui.EventUsageUpdated},
		{name: "run finished", in: agent.EventRunFinished, want: tui.EventRunFinished},
		{name: "unknown", in: agent.EventType("custom"), want: tui.EventType("custom")},
	}
	for _, tc := range cases {
		if got := mapAgentEventType(tc.in); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}

	in := agent.Event{
		Type:          agent.EventToolCallCompleted,
		SessionID:     "sess-1",
		UserInput:     "input",
		Content:       "content",
		ToolName:      "read_file",
		ToolArguments: `{"path":"main.go"}`,
		ToolResult:    "ok",
		Error:         "",
		Plan: planpkg.State{
			Phase: planpkg.PhaseExecuting,
		},
		Usage: llm.Usage{
			InputTokens:   1,
			OutputTokens:  2,
			ContextTokens: 3,
			TotalTokens:   6,
		},
	}
	out := mapAgentEvent(in)
	if out.Type != tui.EventToolCallCompleted {
		t.Fatalf("expected mapped event type %q, got %q", tui.EventToolCallCompleted, out.Type)
	}
	if out.SessionID != "sess-1" || out.ToolName != "read_file" || out.ToolArguments == "" {
		t.Fatalf("expected mapped event fields to be preserved, got %+v", out)
	}
	if out.Usage.TotalTokens != 6 || out.Plan.Phase != planpkg.PhaseExecuting {
		t.Fatalf("expected usage and plan fields to be preserved, got %+v", out)
	}
}

func hasEventType(events []tui.Event, target tui.EventType) bool {
	for _, event := range events {
		if event.Type == target {
			return true
		}
	}
	return false
}
