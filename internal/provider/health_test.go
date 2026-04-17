package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthCheckerStateMachineTransitions(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 2, RecoverProbeSec: 10, RecoverSuccessThreshold: 2, WindowSize: 4}, nil).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeTimeout, Retryable: true})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusDegraded || snapshot.FailureCount != 1 {
		t.Fatalf("unexpected degraded snapshot %#v", snapshot)
	}
	now = now.Add(time.Second)
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusUnavailable || snapshot.FailureCount != 2 || snapshot.NextProbeAt.IsZero() {
		t.Fatalf("unexpected unavailable snapshot %#v", snapshot)
	}
	if err := checker.Check(context.Background(), "openai"); err == nil {
		t.Fatal("expected unavailable before probe window")
	}
	now = snapshot.NextProbeAt
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected half-open probe to pass without checker, got %v", err)
	}
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen {
		t.Fatalf("expected half_open after probe, got %#v", snapshot)
	}
	checker.RecordSuccess(context.Background(), "openai")
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen || snapshot.SuccessCount != 1 {
		t.Fatalf("unexpected half-open recovery snapshot %#v", snapshot)
	}
	checker.RecordSuccess(context.Background(), "openai")
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy || snapshot.SuccessCount != 2 {
		t.Fatalf("unexpected healthy snapshot %#v", snapshot)
	}
}

func TestHealthCheckerIgnoresNonAvailabilityFailures(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1}, nil)
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnauthorized})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy || snapshot.FailureCount != 0 {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
}

func TestHealthCheckerCheckUsesProbeAndRecordsResults(t *testing.T) {
	calls := 0
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 5, RecoverSuccessThreshold: 1}, func(_ context.Context, id ProviderID) error {
		calls++
		if id != "openai" {
			t.Fatalf("unexpected provider %q", id)
		}
		if calls == 1 {
			return &Error{Code: ErrCodeTimeout, Retryable: true}
		}
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	if err := checker.Check(context.Background(), "openai"); err == nil {
		t.Fatal("expected first check failure")
	}
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusUnavailable {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	if err := checker.Check(context.Background(), "openai"); err == nil {
		t.Fatal("expected probe window block")
	}
	now = snapshot.NextProbeAt
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected second probe success, got %v", err)
	}
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy {
		t.Fatalf("unexpected recovered snapshot %#v", snapshot)
	}
}

func TestHealthSchedulerTick(t *testing.T) {
	var checked []ProviderID
	scheduler := NewHealthScheduler(NewHealthChecker(HealthConfig{}, func(_ context.Context, id ProviderID) error {
		checked = append(checked, id)
		return nil
	}), func(context.Context) ([]ProviderID, error) {
		return []ProviderID{"openai", "anthropic"}, nil
	}, HealthConfig{CheckIntervalSec: 1})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(checked) != 2 {
		t.Fatalf("unexpected checked ids %#v", checked)
	}
}

func TestCountsTowardAvailability(t *testing.T) {
	if countsTowardAvailability(nil) || countsTowardAvailability(context.Canceled) {
		t.Fatal("expected nil/canceled to be ignored")
	}
	if countsTowardAvailability(&Error{Code: ErrCodeBadRequest}) {
		t.Fatal("expected bad request to be ignored")
	}
	if !countsTowardAvailability(errors.New("boom")) || !countsTowardAvailability(&Error{Code: ErrCodeRateLimited, Retryable: true}) {
		t.Fatal("expected generic/retryable errors to count")
	}
}
