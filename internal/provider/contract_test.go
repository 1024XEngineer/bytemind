package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type contractStubCompatClient struct {
	message llm.Message
	err     error
	deltas  []string
}

func (s contractStubCompatClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	return s.message, s.err
}

func (s contractStubCompatClient) StreamMessage(_ context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if s.err != nil {
		return llm.Message{}, s.err
	}
	if onDelta != nil {
		for _, delta := range s.deltas {
			onDelta(delta)
		}
	}
	return s.message, nil
}

func collectContractEvents(t *testing.T, client Client, req Request) []Event {
	t.Helper()
	stream, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func assertContractEventEnvelope(t *testing.T, events []Event, providerID ProviderID, modelID ModelID, traceID string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("expected events")
	}
	if events[0].Type != EventStart {
		t.Fatalf("expected start event first, got %#v", events[0])
	}
	terminalCount := 0
	for i, event := range events {
		if event.ID == "" {
			t.Fatalf("expected event %d to have id", i)
		}
		if event.TraceID != traceID {
			t.Fatalf("expected trace %q on event %d, got %#v", traceID, i, event)
		}
		if event.ProviderID != providerID {
			t.Fatalf("expected provider %q on event %d, got %#v", providerID, i, event)
		}
		if event.ModelID != modelID {
			t.Fatalf("expected model %q on event %d, got %#v", modelID, i, event)
		}
		if event.Type == EventResult || event.Type == EventError {
			terminalCount++
			if i != len(events)-1 {
				t.Fatalf("expected terminal event last, got %#v", events)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("expected exactly one terminal event, got %#v", events)
	}
}

func TestProviderContractWrapClientSuccessMatrix(t *testing.T) {
	tests := []struct {
		name       string
		providerID ProviderID
		modelID    ModelID
		client     llm.Client
		assert     func(*testing.T, []Event)
	}{
		{
			name:       "openai compatible preserves normalized success contract",
			providerID: ProviderOpenAI,
			modelID:    "gpt-5.4",
			client: contractStubCompatClient{
				deltas: []string{"hello", " world"},
				message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "hello world",
					Usage:   &llm.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
					ToolCalls: []llm.ToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: llm.ToolFunctionCall{Name: "list_files", Arguments: "{}"},
					}},
				},
			},
			assert: func(t *testing.T, events []Event) {
				assertContractEventEnvelope(t, events, ProviderOpenAI, "gpt-5.4", "trace-openai")
				if len(events) != 6 {
					t.Fatalf("expected start + 2 delta + usage + tool_call + result, got %#v", events)
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
					t.Fatalf("unexpected result event %#v", events[5])
				}
			},
		},
		{
			name:       "anthropic preserves normalized success contract",
			providerID: ProviderAnthropic,
			modelID:    "claude-sonnet",
			client: contractStubCompatClient{
				deltas: []string{"plan ready"},
				message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "plan ready",
					Usage:   &llm.Usage{InputTokens: 10, OutputTokens: 6, TotalTokens: 16},
				},
			},
			assert: func(t *testing.T, events []Event) {
				assertContractEventEnvelope(t, events, ProviderAnthropic, "claude-sonnet", "trace-anthropic")
				if len(events) != 4 {
					t.Fatalf("expected start + delta + usage + result, got %#v", events)
				}
				if events[1].Type != EventDelta || events[1].Delta != "plan ready" {
					t.Fatalf("unexpected delta %#v", events[1])
				}
				if events[2].Type != EventUsage || events[2].Usage == nil || events[2].Usage.TotalTokens != 16 {
					t.Fatalf("unexpected usage %#v", events[2])
				}
				if events[3].Type != EventResult || events[3].Result == nil || events[3].Result.Content != "plan ready" {
					t.Fatalf("unexpected result %#v", events[3])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceID := "trace-openai"
			if tt.providerID == ProviderAnthropic {
				traceID = "trace-anthropic"
			}
			adapter := WrapClient(tt.providerID, tt.modelID, tt.client)
			events := collectContractEvents(t, adapter, Request{ChatRequest: llm.ChatRequest{Model: string(tt.modelID)}, TraceID: traceID})
			tt.assert(t, events)
		})
	}
}

func TestProviderContractWrapClientErrorMatrix(t *testing.T) {
	tests := []struct {
		name       string
		providerID ProviderID
		modelID    ModelID
		err        error
		code       ErrorCode
		retryable  bool
		message    string
	}{
		{
			name:       "openai 429 maps to rate limited",
			providerID: ProviderOpenAI,
			modelID:    "gpt-5.4",
			err:        &llm.ProviderError{Code: llm.ErrorCodeRateLimited, Status: 429, Message: "slow down"},
			code:       ErrCodeRateLimited,
			retryable:  true,
			message:    "provider rate limited",
		},
		{
			name:       "anthropic unauthorized maps to non-retryable",
			providerID: ProviderAnthropic,
			modelID:    "claude-sonnet",
			err:        &llm.ProviderError{Status: 401, Message: "bad auth"},
			code:       ErrCodeUnauthorized,
			retryable:  false,
			message:    "provider unauthorized",
		},
		{
			name:       "timeout maps to retryable timeout",
			providerID: ProviderOpenAI,
			modelID:    "gpt-5.4",
			err:        context.DeadlineExceeded,
			code:       ErrCodeTimeout,
			retryable:  true,
			message:    "provider request timed out",
		},
		{
			name:       "unknown failure maps to unavailable",
			providerID: ProviderAnthropic,
			modelID:    "claude-sonnet",
			err:        errors.New("raw upstream body"),
			code:       ErrCodeUnavailable,
			retryable:  true,
			message:    "provider unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := WrapClient(tt.providerID, tt.modelID, contractStubCompatClient{err: tt.err})
			events := collectContractEvents(t, adapter, Request{ChatRequest: llm.ChatRequest{Model: string(tt.modelID)}, TraceID: "trace-error"})
			assertContractEventEnvelope(t, events, tt.providerID, tt.modelID, "trace-error")
			last := events[len(events)-1]
			if last.Type != EventError || last.Error == nil {
				t.Fatalf("expected terminal error event, got %#v", last)
			}
			if last.Error.Code != tt.code || last.Error.Retryable != tt.retryable || last.Error.Message != tt.message {
				t.Fatalf("unexpected mapped error %#v", last.Error)
			}
		})
	}
}

func TestProviderContractNormalizerMatrix(t *testing.T) {
	tests := []struct {
		name    string
		events  []Event
		assert  func(*testing.T, []Event)
	}{
		{
			name: "duplicate start becomes terminal error",
			events: []Event{{Type: EventStart}, {Type: EventStart}},
			assert: func(t *testing.T, events []Event) {
				if len(events) != 2 || events[1].Type != EventError || events[1].Error == nil || events[1].Error.Code != ErrCodeUnavailable {
					t.Fatalf("unexpected events %#v", events)
				}
			},
		},
		{
			name: "unknown event type becomes terminal error",
			events: []Event{{Type: EventStart}, {Type: EventType("mystery")}},
			assert: func(t *testing.T, events []Event) {
				if len(events) != 2 || events[1].Type != EventError || events[1].Error == nil {
					t.Fatalf("unexpected events %#v", events)
				}
			},
		},
		{
			name: "error without payload becomes terminal error",
			events: []Event{{Type: EventStart}, {Type: EventError}},
			assert: func(t *testing.T, events []Event) {
				if len(events) != 2 || events[1].Type != EventError || events[1].Error == nil {
					t.Fatalf("unexpected events %#v", events)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ch := make(chan Event, len(tt.events)+1)
			normalizer := newStreamNormalizer("trace-normalize", ProviderOpenAI, "gpt-5.4")
			for _, event := range tt.events {
				if !normalizeEvent(ctx, normalizer, ch, event) {
					break
				}
			}
			close(ch)
			var got []Event
			for event := range ch {
				got = append(got, event)
			}
			assertContractEventEnvelope(t, got, ProviderOpenAI, "gpt-5.4", "trace-normalize")
			tt.assert(t, got)
		})
	}
}
