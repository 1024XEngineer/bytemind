package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type staticCompatRouter struct {
	result RouteResult
	err    error
}

func (r staticCompatRouter) Route(context.Context, ModelID, RouteContext) (RouteResult, error) {
	return r.result, r.err
}

type captureCompatRouter struct {
	result      RouteResult
	lastContext RouteContext
	lastModelID ModelID
}

func (r *captureCompatRouter) Route(_ context.Context, model ModelID, rc RouteContext) (RouteResult, error) {
	r.lastModelID = model
	r.lastContext = rc
	return r.result, nil
}

type stubCompatClient struct {
	message llm.Message
	err     error
}

func (s stubCompatClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	return s.message, s.err
}

func (s stubCompatClient) StreamMessage(ctx context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if s.err != nil {
		return llm.Message{}, s.err
	}
	if onDelta != nil {
		onDelta("hello")
		onDelta(" world")
	}
	return s.message, nil
}

type asyncDeltaCompatClient struct {
	message llm.Message
	release chan struct{}
	done    chan struct{}
}

func (s asyncDeltaCompatClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	return s.message, nil
}

func (s asyncDeltaCompatClient) StreamMessage(_ context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if onDelta == nil {
		return s.message, nil
	}
	onDelta("hello")
	go func() {
		<-s.release
		onDelta(" late")
		close(s.done)
	}()
	return s.message, nil
}

func TestWrapClientStreamIgnoresAsyncDeltaAfterTerminal(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{})
	adapter := WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), asyncDeltaCompatClient{message: llm.Message{Role: llm.RoleAssistant, Content: "hello"}, release: release, done: done})
	stream, err := adapter.Stream(context.Background(), Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}, TraceID: "trace-async"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
		if event.Type == EventResult {
			close(release)
			<-done
		}
	}
	if len(events) != 3 {
		t.Fatalf("expected start, delta, result only, got %#v", events)
	}
	if events[0].Type != EventStart || events[1].Type != EventDelta || events[2].Type != EventResult {
		t.Fatalf("unexpected event order %#v", events)
	}
	if events[1].Delta != "hello" {
		t.Fatalf("unexpected delta %#v", events[1])
	}
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
	for i, event := range events {
		if event.ID == "" {
			t.Fatalf("expected event %d to have id, got %#v", i, event)
		}
		if event.ProviderID != ProviderOpenAI {
			t.Fatalf("expected provider normalization on event %d, got %#v", i, event)
		}
		if event.ModelID != "gpt-5.4" {
			t.Fatalf("expected model normalization on event %d, got %#v", i, event)
		}
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
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %#v", events)
	}
	if events[0].Type != EventStart {
		t.Fatalf("expected start event, got %#v", events[0])
	}
	if events[1].Type != EventError || events[1].Error == nil {
		t.Fatalf("expected error event, got %#v", events[1])
	}
	if events[1].Error.Code != ErrCodeUnavailable {
		t.Fatalf("unexpected error code %#v", events[1].Error)
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
			name: "unauthorized",
			err:  &llm.ProviderError{Code: llm.ErrorCodeUnknown, Provider: "openai", Status: 401, Retryable: true, Message: "bad auth"},
			code: ErrCodeUnauthorized, retryable: false, message: "provider unauthorized",
		},
		{
			name: "context too long",
			err:  &llm.ProviderError{Code: llm.ErrorCodeContextTooLong, Provider: "anthropic", Status: 413, Retryable: false, Message: "prompt is too long with raw details"},
			code: ErrCodeBadRequest, retryable: false, message: "request exceeds provider context limit",
		},
		{
			name: "bad request",
			err:  &llm.ProviderError{Code: llm.ErrorCodeUnknown, Provider: "openai", Status: 400, Retryable: true, Message: "invalid payload"},
			code: ErrCodeBadRequest, retryable: false, message: "provider rejected request",
		},
		{
			name: "gateway timeout",
			err:  &llm.ProviderError{Code: llm.ErrorCodeUnknown, Provider: "openai", Status: 504, Retryable: false, Message: "gateway timeout"},
			code: ErrCodeTimeout, retryable: true, message: "provider request timed out",
		},
		{
			name: "fallback unavailable",
			err:  errors.New("sensitive raw body"),
			code: ErrCodeUnavailable, retryable: true, message: "provider unavailable",
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

func TestWrapClientStreamDoesNotEmitNilErrorOnContextCanceled(t *testing.T) {
	adapter := WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{err: context.Canceled})
	stream, err := adapter.Stream(context.Background(), Request{ChatRequest: llm.ChatRequest{Model: "gpt-5.4"}, TraceID: "trace-cancel-err"})
	if err != nil {
		t.Fatalf("expected no setup error, got %v", err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != EventStart {
		t.Fatalf("expected only start event before cancel shutdown, got %#v", events)
	}
}

func TestNormalizeEventRejectsMissingStart(t *testing.T) {
	ctx := context.Background()
	ch := make(chan Event, 1)
	normalizer := newStreamNormalizer("trace-x", ProviderOpenAI, ModelID("gpt-5.4"))
	if !normalizeEvent(ctx, normalizer, ch, Event{Type: EventDelta, Delta: "oops"}) {
		t.Fatal("expected normalized error event to be emitted")
	}
	close(ch)
	events := make([]Event, 0, len(ch))
	for event := range ch {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != EventError || events[0].Error == nil {
		t.Fatalf("expected single error event, got %#v", events)
	}
	if events[0].Error.Code != ErrCodeUnavailable {
		t.Fatalf("expected unavailable normalization error, got %#v", events[0])
	}
}

func TestNormalizeEventRejectsEventsAfterTerminal(t *testing.T) {
	ctx := context.Background()
	ch := make(chan Event, 3)
	normalizer := newStreamNormalizer("trace-y", ProviderOpenAI, ModelID("gpt-5.4"))
	if !normalizeEvent(ctx, normalizer, ch, Event{Type: EventStart}) {
		t.Fatal("expected start event")
	}
	if !normalizeEvent(ctx, normalizer, ch, Event{Type: EventResult, Result: &llm.Message{Role: llm.RoleAssistant}}) {
		t.Fatal("expected result event")
	}
	if normalizeEvent(ctx, normalizer, ch, Event{Type: EventDelta, Delta: "late"}) {
		t.Fatal("expected event after terminal to be rejected")
	}
	close(ch)
	var events []Event
	for event := range ch {
		events = append(events, event)
	}
	if len(events) != 2 || events[0].Type != EventStart || events[1].Type != EventResult {
		t.Fatalf("unexpected normalized events %#v", events)
	}
}

func TestRoutedClientCreateMessageReturnsResultContent(t *testing.T) {
	target := RouteTarget{
		ProviderID: ProviderOpenAI,
		ModelID:    ModelID("gpt-5.4"),
		Client: WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Task complete.",
		}}),
	}
	client := &RoutedClient{router: staticCompatRouter{result: RouteResult{Primary: target}}}
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if msg.Content != "Task complete." {
		t.Fatalf("unexpected message %#v", msg)
	}
}

func TestRoutedClientPreservesRouteContextAndMergesAllowFallback(t *testing.T) {
	target := RouteTarget{
		ProviderID: ProviderOpenAI,
		ModelID:    ModelID("gpt-5.4"),
		Client: WrapClient(ProviderOpenAI, ModelID("gpt-5.4"), stubCompatClient{
			message: llm.Message{Role: llm.RoleAssistant, Content: "ok"},
		}),
	}
	router := &captureCompatRouter{result: RouteResult{Primary: target}}
	client := &RoutedClient{router: router, allowFallback: false}
	ctx := WithRouteContext(context.Background(), RouteContext{
		Scenario:      "incident",
		Region:        "us",
		PreferLatency: true,
		AllowFallback: true,
		Tags: map[string]string{
			"team": "platform",
		},
	})
	_, err := client.CreateMessage(ctx, llm.ChatRequest{Model: "gpt-5.4"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if router.lastModelID != "gpt-5.4" {
		t.Fatalf("expected routed model id to be preserved, got %q", router.lastModelID)
	}
	if router.lastContext.Scenario != "incident" || router.lastContext.Region != "us" || !router.lastContext.PreferLatency {
		t.Fatalf("expected route context metadata to be preserved, got %#v", router.lastContext)
	}
	if router.lastContext.Tags["team"] != "platform" {
		t.Fatalf("expected route context tags to be preserved, got %#v", router.lastContext)
	}
	if !router.lastContext.AllowFallback {
		t.Fatalf("expected allow_fallback to use merge semantics, got %#v", router.lastContext)
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
	if mapped.Code != ErrCodeUnavailable || !mapped.Retryable || mapped.Message != "provider unavailable" || mapped.Provider != ProviderAnthropic || mapped.Detail != "hidden upstream body" {
		t.Fatalf("unexpected mapped error %#v", mapped)
	}
}
