package agent

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/session"
)

type stubEngine struct {
	requests []TurnRequest
	events   []TurnEvent
	err      error
}

func (s *stubEngine) HandleTurn(_ context.Context, req TurnRequest) (<-chan TurnEvent, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	ch := make(chan TurnEvent, len(s.events))
	for _, event := range s.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type engineFunc func(context.Context, TurnRequest) (<-chan TurnEvent, error)

func (f engineFunc) HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error) {
	return f(ctx, req)
}

func TestRunPromptWithInputDelegatesToEngine(t *testing.T) {
	workspace := t.TempDir()
	engine := &stubEngine{
		events: []TurnEvent{
			{Type: TurnEventStarted},
			{Type: TurnEventCompleted, Answer: "delegated answer"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Engine:    engine,
		Stdin:     nil,
		Stdout:    io.Discard,
	})

	sess := session.New(workspace)
	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err != nil {
		t.Fatalf("RunPromptWithInput failed: %v", err)
	}
	if answer != "delegated answer" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(engine.requests) != 1 {
		t.Fatalf("expected one engine request, got %d", len(engine.requests))
	}
	if engine.requests[0].Session != sess {
		t.Fatal("expected runner to pass session through to engine")
	}
	if engine.requests[0].Mode != "build" {
		t.Fatalf("expected mode to be forwarded, got %q", engine.requests[0].Mode)
	}
	if engine.requests[0].Input.DisplayText != "hello" {
		t.Fatalf("expected input to be forwarded, got %q", engine.requests[0].Input.DisplayText)
	}
}

func TestRunPromptWithInputPropagatesEngineFailureEvent(t *testing.T) {
	workspace := t.TempDir()
	engine := &stubEngine{
		events: []TurnEvent{
			{Type: TurnEventStarted},
			{Type: TurnEventFailed, Error: errors.New("turn failed")},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Engine:    engine,
		Stdout:    io.Discard,
	})

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err == nil || err.Error() != "turn failed" {
		t.Fatalf("expected propagated engine failure, got %v", err)
	}
}

func TestRunPromptWithInputPropagatesHandleTurnError(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			return nil, errors.New("handle error")
		}),
		Stdout: io.Discard,
	})

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err == nil || err.Error() != "handle error" {
		t.Fatalf("expected immediate HandleTurn error, got %v", err)
	}
}

func TestRunPromptWithInputFailsWhenEngineReturnsNilEventStream(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			return nil, nil
		}),
		Stdout: io.Discard,
	})

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err == nil || err.Error() != "engine returned nil event stream" {
		t.Fatalf("expected nil stream guard error, got %v", err)
	}
}

func TestRunPromptWithInputFailsWhenEngineClosesWithoutTerminalEvent(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			ch := make(chan TurnEvent, 1)
			ch <- TurnEvent{Type: TurnEventStarted}
			close(ch)
			return ch, nil
		}),
		Stdout: io.Discard,
	})

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err == nil || err.Error() != "engine ended without terminal event" {
		t.Fatalf("expected missing terminal event error, got %v", err)
	}
}

func TestRunPromptWithInputFailsWithGenericErrorWhenFailureEventHasNoCause(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			ch := make(chan TurnEvent, 2)
			ch <- TurnEvent{Type: TurnEventStarted}
			ch <- TurnEvent{Type: TurnEventFailed}
			close(ch)
			return ch, nil
		}),
		Stdout: io.Discard,
	})

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err == nil || err.Error() != "agent turn failed" {
		t.Fatalf("expected generic failed-event error, got %v", err)
	}
}

func TestRunPromptWithInputReturnsContextErrorWhenEngineStalls(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			return make(chan TurnEvent), nil
		}),
		Stdout: io.Discard,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sess := session.New(workspace)
	_, err := runner.RunPromptWithInput(ctx, sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestRunPromptWithInputPrefersCompletedEventWhenCtxDoneAlsoReady(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Engine: engineFunc(func(context.Context, TurnRequest) (<-chan TurnEvent, error) {
			ch := make(chan TurnEvent, 1)
			ch <- TurnEvent{Type: TurnEventCompleted, Answer: "done"}
			close(ch)
			return ch, nil
		}),
		Stdout: io.Discard,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sess := session.New(workspace)
	answer, err := runner.RunPromptWithInput(ctx, sess, RunPromptInput{
		DisplayText: "hello",
	}, "build", io.Discard)
	if err != nil {
		t.Fatalf("expected completed event to win race, got error: %v", err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
}
