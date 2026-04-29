package agent

import (
	"io"
	"strings"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/session"
)

// TurnRequest defines the minimum input needed to execute one agent turn.
type TurnRequest struct {
	Session *session.Session
	Input   RunPromptInput
	Mode    string
	Out     io.Writer
	TraceID corepkg.TraceID
}

// TurnEventType identifies engine turn event categories.
type TurnEventType string

const (
	TurnEventStart      TurnEventType = "start"
	TurnEventDelta      TurnEventType = "delta"
	TurnEventToolUse    TurnEventType = "tool_use"
	TurnEventToolResult TurnEventType = "tool_result"
	TurnEventComplete   TurnEventType = "complete"
	TurnEventError      TurnEventType = "error"

	// Compatibility aliases kept during Runner->Engine migration.
	TurnEventStarted   = TurnEventStart
	TurnEventCompleted = TurnEventComplete
	TurnEventFailed    = TurnEventError
)

// TurnEvent is the normalized turn event contract emitted by Engine.
type TurnEvent struct {
	EventID   corepkg.EventID
	SessionID corepkg.SessionID
	TaskID    corepkg.TaskID
	TraceID   corepkg.TraceID
	TurnID    string
	Sequence  uint64
	Type      TurnEventType
	Timestamp time.Time
	Payload   map[string]any
	ErrorCode string
	Retryable bool

	// Compatibility fields for current runner adapter flow.
	Answer string
	Error  error
}

func (e TurnEvent) IsTerminal() bool {
	switch e.Type {
	case TurnEventComplete, TurnEventError:
		return true
	default:
		return false
	}
}

func ValidateTurnEvent(event TurnEvent) error {
	if strings.TrimSpace(string(event.Type)) == "" {
		return errTurnEventTypeRequired
	}
	if strings.TrimSpace(event.TurnID) == "" {
		return errTurnEventTurnIDRequired
	}
	if event.Sequence == 0 {
		return errTurnEventSequenceRequired
	}
	if event.Timestamp.IsZero() {
		return errTurnEventTimestampRequired
	}
	if strings.TrimSpace(string(event.SessionID)) == "" {
		return errTurnEventSessionIDRequired
	}
	if strings.TrimSpace(string(event.TraceID)) == "" {
		return errTurnEventTraceIDRequired
	}
	return nil
}
