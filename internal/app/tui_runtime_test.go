package app

import (
	notifypkg "bytemind/internal/notify"
	"context"
	"errors"
	"testing"
	"time"
)

func TestResolveTUIStartupPolicyInteractiveDisablesGuideAndAPIKeyRequirement(t *testing.T) {
	guide, requireAPIKey := resolveTUIStartupPolicy(true)
	if guide.Active {
		t.Fatal("expected startup guide to stay disabled for interactive tui")
	}
	if requireAPIKey {
		t.Fatal("expected interactive tui to allow startup without API key")
	}
}

func TestResolveTUIStartupPolicyNonInteractiveRequiresAPIKey(t *testing.T) {
	guide, requireAPIKey := resolveTUIStartupPolicy(false)
	if guide.Active {
		t.Fatal("expected startup guide to stay disabled for non-interactive tui")
	}
	if !requireAPIKey {
		t.Fatal("expected non-interactive tui to require API key")
	}
}

type stubRuntimeNotifier struct {
	closeFn func(context.Context) error
}

func (stubRuntimeNotifier) Notify(notifypkg.Message) {}

func (s stubRuntimeNotifier) Close(ctx context.Context) error {
	if s.closeFn == nil {
		return nil
	}
	return s.closeFn(ctx)
}

func TestChainTUIRuntimeCloseReturnsRunnerErrorFirst(t *testing.T) {
	runnerErr := errors.New("runner close failed")
	notifierErr := errors.New("notifier close failed")
	closeFn := chainTUIRuntimeClose(
		func() error { return runnerErr },
		stubRuntimeNotifier{
			closeFn: func(context.Context) error { return notifierErr },
		},
	)

	err := closeFn()
	if !errors.Is(err, runnerErr) {
		t.Fatalf("expected runner error to have priority, got %v", err)
	}
}

func TestChainTUIRuntimeCloseReturnsNotifierError(t *testing.T) {
	notifierErr := errors.New("notifier close failed")
	closeFn := chainTUIRuntimeClose(
		nil,
		stubRuntimeNotifier{
			closeFn: func(context.Context) error { return notifierErr },
		},
	)

	err := closeFn()
	if !errors.Is(err, notifierErr) {
		t.Fatalf("expected notifier close error, got %v", err)
	}
}

func TestChainTUIRuntimeCloseUsesFixedNotifierTimeout(t *testing.T) {
	closeFn := chainTUIRuntimeClose(
		nil,
		stubRuntimeNotifier{
			closeFn: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
	)

	start := time.Now()
	err := closeFn()
	if err == nil {
		t.Fatalf("expected timeout error from notifier close")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < tuiRuntimeNotifierCloseTimeout/2 || elapsed > tuiRuntimeNotifierCloseTimeout*3 {
		t.Fatalf("expected timeout close to be near %s, got %s", tuiRuntimeNotifierCloseTimeout, elapsed)
	}
}
