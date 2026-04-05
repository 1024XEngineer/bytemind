package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

type recordingClient struct {
	replies  []llm.Message
	index    int
	requests []llm.ChatRequest
}

func (c *recordingClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
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

func (c *recordingClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	reply, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && reply.Content != "" {
		onDelta(reply.Content)
	}
	return reply, nil
}

type fakeTool struct {
	name string
	run  func(raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error)
}

func (t *fakeTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: t.name,
		},
	}
}

func (t *fakeTool) Run(_ context.Context, raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
	return t.run(raw, execCtx)
}

func TestRunPromptStreamsAssistantReplyAndEmitsObserverEvents(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "Streamed final answer.",
	}}}
	var out bytes.Buffer
	events := make([]Event, 0, 4)

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        true,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "say hi", "build", &out)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Streamed final answer." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if got := out.String(); !strings.Contains(got, "Streamed final answer.") {
		t.Fatalf("expected streamed output, got %q", got)
	}

	eventTypes := make([]EventType, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.Type)
	}
	wantTypes := []EventType{
		EventRunStarted,
		EventAssistantDelta,
		EventAssistantMessage,
		EventRunFinished,
	}
	if !slices.Equal(eventTypes, wantTypes) {
		t.Fatalf("unexpected event sequence: got=%v want=%v", eventTypes, wantTypes)
	}
	if events[1].Content != "Streamed final answer." {
		t.Fatalf("expected assistant delta content, got %#v", events[1])
	}
}

func TestRunPromptExecutesMultipleToolCallsInOrder(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	executionOrder := make([]string, 0, 2)
	firstTool := &fakeTool{
		name: "first_tool",
		run: func(raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
			executionOrder = append(executionOrder, "first_tool")
			return `{"ok":true,"step":"first"}`, nil
		},
	}
	secondTool := &fakeTool{
		name: "second_tool",
		run: func(raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
			executionOrder = append(executionOrder, "second_tool")
			return `{"ok":true,"step":"second"}`, nil
		},
	}

	registry := tools.DefaultRegistry()
	registry.Add(firstTool)
	registry.Add(secondTool)

	client := &recordingClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "first_tool",
						Arguments: `{"step":1}`,
					},
				},
				{
					ID:   "call-2",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "second_tool",
						Arguments: `{"step":2}`,
					},
				},
			},
		},
		{
			Role:    "assistant",
			Content: "Done with both tools.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "do both steps", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Done with both tools." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if !slices.Equal(executionOrder, []string{"first_tool", "second_tool"}) {
		t.Fatalf("unexpected tool execution order: %v", executionOrder)
	}
	if len(sess.Messages) != 5 {
		t.Fatalf("expected 5 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[2].ToolCallID != "call-1" || sess.Messages[3].ToolCallID != "call-2" {
		t.Fatalf("expected tool results in order, got %#v %#v", sess.Messages[2], sess.Messages[3])
	}
}

func TestRunPromptUpdatePlanSyncsSessionAndEmitsPlanEvent(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-plan",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "update_plan",
					Arguments: `{"explanation":"starting","plan":[{"step":"Inspect provider","status":"completed"},{"step":"Add tests","status":"in_progress"}]}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Plan updated.",
		},
	}}

	events := make([]Event, 0, 6)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "make a plan", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Plan updated." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Plan.Steps) != 2 || sess.Plan.Steps[1].Title != "Add tests" || sess.Plan.Steps[1].Status != planpkg.StepInProgress {
		t.Fatalf("unexpected session plan: %#v", sess.Plan)
	}

	eventTypes := make([]EventType, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.Type)
	}
	wantTypes := []EventType{
		EventRunStarted,
		EventToolCallStarted,
		EventToolCallCompleted,
		EventPlanUpdated,
		EventAssistantMessage,
		EventRunFinished,
	}
	if !slices.Equal(eventTypes, wantTypes) {
		t.Fatalf("unexpected event sequence: got=%v want=%v", eventTypes, wantTypes)
	}
	if len(events[3].Plan.Steps) != 2 || events[3].Plan.Steps[1].Title != "Add tests" {
		t.Fatalf("expected plan in event, got %#v", events[3])
	}
}

func TestRunPromptSystemPromptIncludesToolCatalog(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("Prefer focused edits."), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-plan",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "update_plan",
					Arguments: `{"plan":[{"step":"Inspect provider","status":"completed"},{"step":"Add tests","status":"in_progress"}]}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Plan acknowledged.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.RunPrompt(context.Background(), sess, "make a plan", "build", io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected two LLM requests, got %d", len(client.requests))
	}
	systemPrompt := client.requests[1].Messages[0].Content
	for _, want := range []string{"[Available Tools]", "list_files", "read_file", "[Instructions]", "Prefer focused edits."} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected second-turn system prompt to include %q, got %q", want, systemPrompt)
		}
	}
}

func TestRunPromptUsesSessionModeWhenModeArgEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModePlan

	client := &recordingClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "Plan draft ready.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.RunPrompt(context.Background(), sess, "draft a plan", "", io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one LLM request, got %d", len(client.requests))
	}
	systemPrompt := client.requests[0].Messages[0].Content
	for _, want := range []string{"[Current Mode]", "plan", "mode: plan"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected system prompt to include %q, got %q", want, systemPrompt)
		}
	}
}

func TestRunPromptInjectsExplicitWebLookupInstruction(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "Checked online sources.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.RunPrompt(context.Background(), sess, "去 GitHub 源码里找这个函数实现", "build", io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(client.requests))
	}
	if len(client.requests[0].Messages) < 2 {
		t.Fatalf("expected at least two system messages, got %#v", client.requests[0].Messages)
	}
	if client.requests[0].Messages[1].Role != "system" {
		t.Fatalf("expected second message to be system hint, got %#v", client.requests[0].Messages[1])
	}
	if !strings.Contains(client.requests[0].Messages[1].Content, "web_search/web_fetch") {
		t.Fatalf("expected web lookup hint in second system message, got %q", client.requests[0].Messages[1].Content)
	}
}
