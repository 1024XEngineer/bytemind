package agent

import (
	"testing"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestObserverFuncHandleEvent(t *testing.T) {
	var receivedEvent Event
	handler := ObserverFunc(func(event Event) {
		receivedEvent = event
	})

	event := Event{
		Type:      EventRunStarted,
		SessionID: corepkg.SessionID("test-session"),
		Content:   "test content",
	}

	handler.HandleEvent(event)

	if receivedEvent.Type != EventRunStarted {
		t.Errorf("expected EventRunStarted, got: %v", receivedEvent.Type)
	}
	if receivedEvent.Content != "test content" {
		t.Errorf("expected 'test content', got: %s", receivedEvent.Content)
	}
}

func TestSubAgentObserver(t *testing.T) {
	t.Run("wraps and tags events with agentID and invocationID", func(t *testing.T) {
		var receivedEvent Event
		inner := ObserverFunc(func(event Event) {
			receivedEvent = event
		})

		wrapped := SubAgentObserver(inner, "subagent-123", "inv-001")

		event := Event{
			Type:      EventAssistantMessage,
			SessionID: corepkg.SessionID("session-1"),
			Content:   "hello",
		}

		wrapped.HandleEvent(event)

		if receivedEvent.AgentID != "subagent-123" {
			t.Errorf("expected AgentID 'subagent-123', got: %s", receivedEvent.AgentID)
		}
		if receivedEvent.InvocationID != "inv-001" {
			t.Errorf("expected InvocationID 'inv-001', got: %s", receivedEvent.InvocationID)
		}
		if receivedEvent.Content != "hello" {
			t.Errorf("expected Content 'hello', got: %s", receivedEvent.Content)
		}
	})

	t.Run("nil inner returns no-op observer", func(t *testing.T) {
		wrapped := SubAgentObserver(nil, "agent-456", "inv-002")

		event := Event{
			Type:    EventRunFinished,
			Content: "test",
		}

		shouldNotPanic := func() (caught bool) {
			defer func() {
				if r := recover(); r != nil {
					caught = true
				}
			}()
			wrapped.HandleEvent(event)
			return false
		}()

		if shouldNotPanic {
			t.Error("expected no panic with nil inner")
		}
	})

	t.Run("empty IDs still work", func(t *testing.T) {
		var receivedEvent Event
		inner := ObserverFunc(func(event Event) {
			receivedEvent = event
		})

		wrapped := SubAgentObserver(inner, "", "")

		event := Event{
			Type:    EventToolCallCompleted,
			Content: "tool result",
		}

		wrapped.HandleEvent(event)

		if receivedEvent.AgentID != "" {
			t.Errorf("expected empty AgentID, got: %s", receivedEvent.AgentID)
		}
		if receivedEvent.InvocationID != "" {
			t.Errorf("expected empty InvocationID, got: %s", receivedEvent.InvocationID)
		}
	})
}

func TestEventTypes(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventRunStarted, "run_started"},
		{EventAssistantDelta, "assistant_delta"},
		{EventAssistantMessage, "assistant_message"},
		{EventToolCallStarted, "tool_call_started"},
		{EventToolCallCompleted, "tool_call_completed"},
		{EventPlanUpdated, "plan_updated"},
		{EventUsageUpdated, "usage_updated"},
		{EventRunFinished, "run_finished"},
	}

	for _, tc := range tests {
		if string(tc.eventType) != tc.expected {
			t.Errorf("expected %q, got: %q", tc.expected, tc.eventType)
		}
	}
}
