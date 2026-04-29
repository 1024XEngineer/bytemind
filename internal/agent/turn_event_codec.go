package agent

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

var (
	errTurnEventTypeRequired      = errors.New("turn event type is required")
	errTurnEventTurnIDRequired    = errors.New("turn event turn_id is required")
	errTurnEventSequenceRequired  = errors.New("turn event sequence must be greater than zero")
	errTurnEventTimestampRequired = errors.New("turn event timestamp is required")
	errTurnEventSessionIDRequired = errors.New("turn event session_id is required")
	errTurnEventTraceIDRequired   = errors.New("turn event trace_id is required")
	errTurnEventAfterTerminal     = errors.New("turn terminal event already emitted")
)

var turnEventCounter atomic.Uint64

type turnEventStream struct {
	mu              sync.Mutex
	channel         chan TurnEvent
	turnID          string
	sessionID       corepkg.SessionID
	traceID         corepkg.TraceID
	sequence        uint64
	terminalEmitted bool
	closed          bool
}

func newTurnEventStream(sessionID corepkg.SessionID, traceID corepkg.TraceID) *turnEventStream {
	if sessionID == "" {
		sessionID = corepkg.SessionID("unknown")
	}
	if traceID == "" {
		traceID = corepkg.TraceID(newTurnScopedID("trace"))
	}
	return &turnEventStream{
		channel:   make(chan TurnEvent, 8),
		turnID:    newTurnScopedID("turn"),
		sessionID: sessionID,
		traceID:   traceID,
	}
}

func (s *turnEventStream) Events() <-chan TurnEvent {
	return s.channel
}

func (s *turnEventStream) Emit(event TurnEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.terminalEmitted {
		return errTurnEventAfterTerminal
	}

	now := time.Now().UTC()
	s.sequence++

	if event.EventID == "" {
		event.EventID = corepkg.EventID(newTurnScopedID("tevt"))
	}
	if event.SessionID == "" {
		event.SessionID = s.sessionID
	}
	if event.TraceID == "" {
		event.TraceID = s.traceID
	}
	if event.TurnID == "" {
		event.TurnID = s.turnID
	}
	// This stream owns sequencing; caller-provided sequence is ignored.
	event.Sequence = s.sequence
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}

	if err := ValidateTurnEvent(event); err != nil {
		return err
	}

	s.channel <- event
	if event.IsTerminal() {
		s.terminalEmitted = true
		close(s.channel)
		s.closed = true
	}
	return nil
}

func (s *turnEventStream) CloseWithoutTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	close(s.channel)
	s.closed = true
}

func newTurnScopedID(prefix string) string {
	n := turnEventCounter.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), n)
}
