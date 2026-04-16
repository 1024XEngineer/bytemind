package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"bytemind/internal/config"
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
