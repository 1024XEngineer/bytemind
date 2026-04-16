package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"bytemind/internal/llm"
)

type stubCompatClient struct {
	message llm.Message
	err     error
}

func (s stubCompatClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	return s.message, s.err
}

func (s stubCompatClient) StreamMessage(_ context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if onDelta != nil {
		onDelta("hello")
		onDelta(" world")
	}
	return s.message, s.err
}

func TestWrapClientStreamEmitsNormalizedEvents(t *testing.T) {
	adapter := WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{message: llm.Message{
		Role:    llm.RoleAssistant,
		Content: "hello world",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: llm.ToolFunctionCall{
				Name:      "list_files",
				Arguments: "{}",
			},
		}},
		Usage: &llm.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
	}})
	stream, err := adapter.Stream(context.Background(), Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}, TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %#v", events)
	}
	if events[0].Type != EventStart || events[0].TraceID != "trace-1" {
		t.Fatalf("unexpected start event %#v", events[0])
	}
	if events[1].Type != EventDelta || events[1].Delta != "hello" {
		t.Fatalf("unexpected first delta %#v", events[1])
	}
	if events[2].Type != EventDelta || events[2].Delta != " world" {
		t.Fatalf("unexpected second delta %#v", events[2])
	}
	if events[3].Type != EventUsage || events[3].Usage == nil || events[3].Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage event %#v", events[3])
	}
	if events[4].Type != EventToolCall || events[4].ToolCall == nil || events[4].ToolCall.Function.Name != "list_files" {
		t.Fatalf("unexpected tool call event %#v", events[4])
	}
	if events[5].Type != EventResult || events[5].Result == nil || events[5].Result.Content != "hello world" {
		t.Fatalf("expected final result event, got %#v", events[5])
	}
}

func TestWrapClientStreamEmitsErrorEvent(t *testing.T) {
	adapter := WrapClient(ProviderAnthropic, ModelID("claude"), stubCompatClient{err: errors.New("boom")})
	stream, err := adapter.Stream(context.Background(), Request{ChatRequest: llm.ChatRequest{}, TraceID: "trace-2"})
	if err != nil {
		t.Fatalf("expected no setup error, got %v", err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %#v", events)
	}
	if events[0].Type != EventStart {
		t.Fatalf("expected start event, got %#v", events[0])
	}
	if events[1].Type != EventDelta || events[2].Type != EventDelta {
		t.Fatalf("expected delta events before error, got %#v", events)
	}
	if events[3].Type != EventError || events[3].Error == nil {
		t.Fatalf("expected error event, got %#v", events[3])
	}
	if events[3].Error.Code != ErrCodeUnavailable {
		t.Fatalf("unexpected error code %#v", events[3].Error)
	}
}

func TestWrapClientStreamMapsProviderErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      ErrorCode
		retryable bool
		message   string
	}{
		{
			name: "rate limited",
			err:  &llm.ProviderError{Code: llm.ErrorCodeRateLimited, Provider: "openai", Status: 429, Retryable: true, Message: "429: raw upstream body"},
			code: ErrCodeRateLimited, retryable: true, message: "provider rate limited",
		},
		{
			name: "context too long",
			err:  &llm.ProviderError{Code: llm.ErrorCodeContextTooLong, Provider: "anthropic", Status: 413, Retryable: false, Message: "prompt is too long with raw details"},
			code: ErrCodeBadRequest, retryable: false, message: "request exceeds provider context limit",
		},
		{
			name: "fallback unavailable",
			err:  errors.New("sensitive raw body"),
			code: ErrCodeUnavailable, retryable: false, message: "provider request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{err: tt.err})
			stream, err := adapter.Stream(context.Background(), Request{ChatRequest: llm.ChatRequest{}, TraceID: "trace-map"})
			if err != nil {
				t.Fatalf("expected no setup error, got %v", err)
			}
			var got *Error
			for event := range stream {
				if event.Type == EventError {
					got = event.Error
				}
			}
			if got == nil {
				t.Fatal("expected error event")
			}
			if got.Code != tt.code || got.Retryable != tt.retryable || got.Provider != ProviderOpenAI || got.Message != tt.message {
				t.Fatalf("unexpected mapped error %#v", got)
			}
		})
	}
}

func TestWrapClientStreamStopsWhenContextCancelled(t *testing.T) {
	adapter := WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{message: llm.Message{Role: llm.RoleAssistant, Content: "hello world"}})
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := adapter.Stream(ctx, Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}, TraceID: "trace-cancel"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	<-stream
	cancel()
	select {
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected stream to stop after cancellation")
	case _, ok := <-stream:
		if ok {
			for range stream {
			}
		}
	}
}

func TestEmitReturnsFalseWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if emit(ctx, make(chan Event), Event{}) {
		t.Fatal("expected emit to fail when context is cancelled")
	}
}

func TestWrapClientCoversNilClientAndEmptyModel(t *testing.T) {
	if WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), nil) != nil {
		t.Fatal("expected nil client to return nil adapter")
	}
	adapter := WrapClient("", ModelID(""), stubCompatClient{})
	if adapter.ProviderID() != ProviderID("unknown") {
		t.Fatalf("unexpected provider id %q", adapter.ProviderID())
	}
	models, err := adapter.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Fatalf("expected nil models, got %#v", models)
	}
}

func TestMapCompatErrorDefaultProviderErrorBranch(t *testing.T) {
	mapped := mapCompatError(ProviderAnthropic, &llm.ProviderError{Code: llm.ErrorCodeUnknown, Retryable: true, Message: "hidden upstream body"})
	if mapped.Code != ErrCodeUnavailable || !mapped.Retryable || mapped.Message != "provider unavailable" || mapped.Provider != ProviderAnthropic {
		t.Fatalf("unexpected mapped error %#v", mapped)
	}
}
