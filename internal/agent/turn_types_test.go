package agent

import (
	"testing"
	"time"
)

func TestTurnEventIsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		event    TurnEvent
		expected bool
	}{
		{name: "start", event: TurnEvent{Type: TurnEventStart}, expected: false},
		{name: "delta", event: TurnEvent{Type: TurnEventDelta}, expected: false},
		{name: "tool_use", event: TurnEvent{Type: TurnEventToolUse}, expected: false},
		{name: "tool_result", event: TurnEvent{Type: TurnEventToolResult}, expected: false},
		{name: "complete", event: TurnEvent{Type: TurnEventComplete}, expected: true},
		{name: "error", event: TurnEvent{Type: TurnEventError}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.IsTerminal(); got != tt.expected {
				t.Fatalf("IsTerminal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidateTurnEventRequiredFields(t *testing.T) {
	base := TurnEvent{
		Type:      TurnEventStart,
		TurnID:    "turn-1",
		Sequence:  1,
		Timestamp: time.Now().UTC(),
		SessionID: "session-1",
		TraceID:   "trace-1",
	}
	if err := ValidateTurnEvent(base); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}

	tests := []struct {
		name string
		mut  func(TurnEvent) TurnEvent
		want error
	}{
		{
			name: "missing_type",
			mut: func(e TurnEvent) TurnEvent {
				e.Type = ""
				return e
			},
			want: errTurnEventTypeRequired,
		},
		{
			name: "missing_turn_id",
			mut: func(e TurnEvent) TurnEvent {
				e.TurnID = ""
				return e
			},
			want: errTurnEventTurnIDRequired,
		},
		{
			name: "missing_sequence",
			mut: func(e TurnEvent) TurnEvent {
				e.Sequence = 0
				return e
			},
			want: errTurnEventSequenceRequired,
		},
		{
			name: "missing_timestamp",
			mut: func(e TurnEvent) TurnEvent {
				e.Timestamp = time.Time{}
				return e
			},
			want: errTurnEventTimestampRequired,
		},
		{
			name: "missing_session_id",
			mut: func(e TurnEvent) TurnEvent {
				e.SessionID = ""
				return e
			},
			want: errTurnEventSessionIDRequired,
		},
		{
			name: "missing_trace_id",
			mut: func(e TurnEvent) TurnEvent {
				e.TraceID = ""
				return e
			},
			want: errTurnEventTraceIDRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.mut(base)
			if err := ValidateTurnEvent(event); err != tt.want {
				t.Fatalf("ValidateTurnEvent() = %v, want %v", err, tt.want)
			}
		})
	}
}
