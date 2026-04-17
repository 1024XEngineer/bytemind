package agent

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

func TestTurnEventStreamMonotonicSequenceAndTerminalClose(t *testing.T) {
	stream := newTurnEventStream("sess-1", "trace-1")
	if err := stream.Emit(TurnEvent{Type: TurnEventStart}); err != nil {
		t.Fatalf("emit start failed: %v", err)
	}
	if err := stream.Emit(TurnEvent{Type: TurnEventComplete, Answer: "done"}); err != nil {
		t.Fatalf("emit complete failed: %v", err)
	}

	var got []TurnEvent
	for event := range stream.Events() {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].Type != TurnEventStart || got[1].Type != TurnEventComplete {
		t.Fatalf("unexpected event order: %#v", got)
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("expected monotonic sequence 1,2 got %d,%d", got[0].Sequence, got[1].Sequence)
	}
	if got[0].TurnID == "" || got[0].TurnID != got[1].TurnID {
		t.Fatalf("expected stable turn id, got %#v", got)
	}
	if got[0].SessionID == "" || got[1].SessionID == "" {
		t.Fatalf("expected session id in all events, got %#v", got)
	}
	if got[0].TraceID == "" || got[1].TraceID == "" {
		t.Fatalf("expected trace id in all events, got %#v", got)
	}
	if got[0].EventID == "" || got[1].EventID == "" {
		t.Fatalf("expected event id in all events, got %#v", got)
	}
	if err := ValidateTurnEvent(got[0]); err != nil {
		t.Fatalf("start event failed validation: %v", err)
	}
	if err := ValidateTurnEvent(got[1]); err != nil {
		t.Fatalf("complete event failed validation: %v", err)
	}
}

func TestTurnEventStreamRejectsEmitAfterTerminal(t *testing.T) {
	stream := newTurnEventStream("sess-1", "trace-1")
	if err := stream.Emit(TurnEvent{Type: TurnEventStart}); err != nil {
		t.Fatalf("emit start failed: %v", err)
	}
	if err := stream.Emit(TurnEvent{Type: TurnEventError, ErrorCode: "boom"}); err != nil {
		t.Fatalf("emit error failed: %v", err)
	}
	if err := stream.Emit(TurnEvent{Type: TurnEventComplete, Answer: "late"}); err != errTurnEventAfterTerminal {
		t.Fatalf("expected terminal guard error, got %v", err)
	}
}

func TestTurnEventStreamOverridesCallerProvidedSequence(t *testing.T) {
	stream := newTurnEventStream("sess-1", "trace-1")
	if err := stream.Emit(TurnEvent{Type: TurnEventStart, Sequence: 42}); err != nil {
		t.Fatalf("emit start failed: %v", err)
	}
	if err := stream.Emit(TurnEvent{Type: TurnEventComplete, Sequence: 7}); err != nil {
		t.Fatalf("emit complete failed: %v", err)
	}

	got := make([]TurnEvent, 0, 2)
	for event := range stream.Events() {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("expected stream-owned sequence 1,2 got %d,%d", got[0].Sequence, got[1].Sequence)
	}
}

func TestTurnEventStreamCloseWithoutTerminal(t *testing.T) {
	stream := newTurnEventStream("sess-1", "trace-1")
	if err := stream.Emit(TurnEvent{Type: TurnEventStart}); err != nil {
		t.Fatalf("emit start failed: %v", err)
	}

	stream.CloseWithoutTerminal()
	// idempotent close should not panic or block
	stream.CloseWithoutTerminal()

	events := make([]TurnEvent, 0, 2)
	for event := range stream.Events() {
		events = append(events, event)
	}
	if len(events) != 1 {
		t.Fatalf("expected one start event before close, got %d", len(events))
	}
	if events[0].Type != TurnEventStart {
		t.Fatalf("expected start event, got %s", events[0].Type)
	}
	if err := stream.Emit(TurnEvent{Type: TurnEventComplete}); err != errTurnEventAfterTerminal {
		t.Fatalf("expected emit-after-close to fail, got %v", err)
	}
}

func TestDefaultEngineHandleTurnEmitsOrderedTerminalEvents(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{Role: "assistant", Content: "engine answer"},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    100000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(context.Background(), TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "hello",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	got := make([]TurnEvent, 0, 4)
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("expected start+terminal event, got %d (%#v)", len(got), got)
	}
	if got[0].Type != TurnEventStart || got[1].Type != TurnEventComplete {
		t.Fatalf("expected start->complete, got %#v", got)
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("unexpected sequence: %d,%d", got[0].Sequence, got[1].Sequence)
	}
	if got[0].TurnID == "" || got[0].TurnID != got[1].TurnID {
		t.Fatalf("expected stable turn id, got %#v", got)
	}
	if got[0].SessionID == "" || got[1].SessionID == "" {
		t.Fatalf("expected session id on all events, got %#v", got)
	}
	if got[0].TraceID == "" || got[1].TraceID == "" {
		t.Fatalf("expected trace id on all events, got %#v", got)
	}
}

func TestDefaultEngineHandleTurnEmitsToolUseAndResultEvents(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	registry := tools.DefaultRegistry()

	client := &recordingClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{"path":".","depth":1,"limit":20}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "engine answer",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
			TokenQuota:    100000,
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(context.Background(), TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "run tool",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	got := make([]TurnEvent, 0, 6)
	for event := range events {
		got = append(got, event)
	}

	types := make([]TurnEventType, 0, len(got))
	for _, event := range got {
		types = append(types, event.Type)
	}
	wantTypes := []TurnEventType{TurnEventStart, TurnEventToolUse, TurnEventToolResult, TurnEventComplete}
	if !slices.Equal(types, wantTypes) {
		t.Fatalf("unexpected event sequence: got=%v want=%v", types, wantTypes)
	}

	toolUse := got[1]
	if toolUse.Payload["tool_name"] != "list_files" {
		t.Fatalf("expected tool_use payload to include tool name, got %#v", toolUse.Payload)
	}
	if toolUse.Payload["tool_call_id"] != "call-1" {
		t.Fatalf("expected tool_use payload to include call id, got %#v", toolUse.Payload)
	}

	toolResult := got[2]
	if toolResult.Payload["tool_name"] != "list_files" {
		t.Fatalf("expected tool_result payload to include tool name, got %#v", toolResult.Payload)
	}
	resultRaw, _ := toolResult.Payload["tool_result"].(string)
	if !strings.Contains(resultRaw, `"ok":true`) {
		t.Fatalf("expected encoded tool result payload, got %#v", toolResult.Payload)
	}
}

func TestDefaultEngineHandleTurnEmitsDeltaEventsForStreaming(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "streamed answer"},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        true,
			TokenQuota:    100000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(context.Background(), TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "hello",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	got := make([]TurnEvent, 0, 4)
	for event := range events {
		got = append(got, event)
	}
	types := make([]TurnEventType, 0, len(got))
	for _, event := range got {
		types = append(types, event.Type)
	}
	wantTypes := []TurnEventType{TurnEventStart, TurnEventDelta, TurnEventComplete}
	if !slices.Equal(types, wantTypes) {
		t.Fatalf("unexpected event sequence: got=%v want=%v", types, wantTypes)
	}
	if got[1].Payload["content"] != "streamed answer" {
		t.Fatalf("unexpected delta payload: %#v", got[1].Payload)
	}
}

type denyAllPolicyGateway struct{}

func (denyAllPolicyGateway) DecideTool(context.Context, ToolDecisionInput) (ToolDecision, error) {
	return ToolDecision{
		Decision:   corepkg.DecisionDeny,
		ReasonCode: policyReasonRiskRule,
		Reason:     "blocked for test",
		RiskLevel:  corepkg.RiskHigh,
	}, nil
}

type cancelAwareClient struct{}

func (cancelAwareClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	<-ctx.Done()
	return llm.Message{}, ctx.Err()
}

func (cancelAwareClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	<-ctx.Done()
	return llm.Message{}, ctx.Err()
}

type panicClient struct {
	panicValue any
}

func (c panicClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	panic(c.panicValue)
}

func (c panicClient) StreamMessage(context.Context, llm.ChatRequest, func(string)) (llm.Message, error) {
	panic(c.panicValue)
}

func TestDefaultEngineHandleTurnEmitsToolResultWhenPolicyDenies(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &recordingClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-denied",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "run_shell",
					Arguments: `{"command":"echo hi"}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done after deny",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
			TokenQuota:    100000,
		},
		Client:        client,
		Store:         store,
		Registry:      tools.DefaultRegistry(),
		PolicyGateway: denyAllPolicyGateway{},
		Stdin:         strings.NewReader(""),
		Stdout:        io.Discard,
	})

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(context.Background(), TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "try denied tool",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	got := make([]TurnEvent, 0, 6)
	for event := range events {
		got = append(got, event)
	}
	types := make([]TurnEventType, 0, len(got))
	for _, event := range got {
		types = append(types, event.Type)
	}
	wantTypes := []TurnEventType{TurnEventStart, TurnEventToolUse, TurnEventToolResult, TurnEventComplete}
	if !slices.Equal(types, wantTypes) {
		t.Fatalf("unexpected event sequence: got=%v want=%v", types, wantTypes)
	}

	toolResult := got[2]
	resultRaw, _ := toolResult.Payload["tool_result"].(string)
	if !strings.Contains(resultRaw, `"ok":false`) {
		t.Fatalf("expected deny result payload, got %#v", toolResult.Payload)
	}
	errText, _ := toolResult.Payload["error"].(string)
	if !strings.Contains(errText, "blocked") {
		t.Fatalf("expected blocked error text, got %#v", toolResult.Payload)
	}
}

func TestDefaultEngineHandleTurnEmitsErrorEventOnContextCancel(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    100000,
		},
		Client:   cancelAwareClient{},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(ctx, TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "cancel test",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	var start TurnEvent
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event stream closed before start event")
		}
		start = event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for start event")
	}
	if start.Type != TurnEventStart {
		t.Fatalf("expected start event, got %#v", start)
	}

	cancel()

	var terminal TurnEvent
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event stream closed before terminal error")
		}
		terminal = event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal error")
	}

	if terminal.Type != TurnEventError {
		t.Fatalf("expected error terminal event, got %#v", terminal)
	}
	if !errors.Is(terminal.Error, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", terminal.Error)
	}
	if terminal.ErrorCode != "run_failed" {
		t.Fatalf("expected run_failed error code, got %q", terminal.ErrorCode)
	}
}

func TestDefaultEngineHandleTurnEmitsErrorEventOnPanic(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    100000,
		},
		Client:   panicClient{panicValue: "panic-from-client"},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	engine := NewDefaultEngine(runner)
	events, err := engine.HandleTurn(context.Background(), TurnRequest{
		Session: sess,
		Input: RunPromptInput{
			DisplayText: "panic test",
		},
		Mode: "build",
		Out:  io.Discard,
	})
	if err != nil {
		t.Fatalf("HandleTurn failed: %v", err)
	}

	got := make([]TurnEvent, 0, 4)
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("expected start+terminal event, got %d (%#v)", len(got), got)
	}
	if got[0].Type != TurnEventStart || got[1].Type != TurnEventError {
		t.Fatalf("expected start->error sequence, got %#v", got)
	}
	if got[1].ErrorCode != "run_panicked" {
		t.Fatalf("expected run_panicked error code, got %q", got[1].ErrorCode)
	}
	if got[1].Error == nil {
		t.Fatalf("expected panic error payload, got %#v", got[1])
	}
	if !strings.Contains(got[1].Error.Error(), "panic-from-client") {
		t.Fatalf("expected panic value in error payload, got %v", got[1].Error)
	}
}
