package agent

import (
	"context"
	"errors"

	corepkg "bytemind/internal/core"
)

// Engine executes one turn and emits turn-scoped events.
//
// Contract:
//   - Implementations should emit exactly one terminal event (TurnEventComplete or TurnEventError).
//   - Implementations should close the channel after the terminal event.
type Engine interface {
	HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error)
}

type defaultEngine struct {
	runner *Runner
}

// NewDefaultEngine wires the default engine implementation backed by Runner internals.
func NewDefaultEngine(runner *Runner) Engine {
	return &defaultEngine{runner: runner}
}

func (e *defaultEngine) HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error) {
	if e == nil || e.runner == nil {
		return nil, errors.New("agent engine is unavailable")
	}

	sessionID := corepkg.SessionID("")
	if req.Session != nil {
		sessionID = corepkg.SessionID(req.Session.ID)
	}
	stream := newTurnEventStream(sessionID, req.TraceID)
	events := stream.Events()

	go func() {
		if err := stream.Emit(TurnEvent{Type: TurnEventStart}); err != nil {
			stream.CloseWithoutTerminal()
			return
		}

		if req.Session == nil {
			_ = stream.Emit(TurnEvent{
				Type:      TurnEventError,
				Error:     errors.New("session is required"),
				ErrorCode: "invalid_session",
			})
			return
		}

		setup, err := e.runner.prepareRunPrompt(req.Session, req.Input, req.Mode)
		if err != nil {
			_ = stream.Emit(TurnEvent{
				Type:      TurnEventError,
				Error:     err,
				ErrorCode: "prepare_failed",
			})
			return
		}

		answer, err := e.runner.runPromptTurns(ctx, req.Session, setup, req.Out)
		if err != nil {
			_ = stream.Emit(TurnEvent{
				Type:      TurnEventError,
				Error:     err,
				ErrorCode: "run_failed",
			})
			return
		}

		_ = stream.Emit(TurnEvent{
			Type:   TurnEventComplete,
			Answer: answer,
		})
	}()

	return events, nil
}
