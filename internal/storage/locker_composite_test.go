package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestCompositeLockerUsesFileLayerAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	fileLocker1, err := NewFileLockerWithPollInterval(dir, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	fileLocker2, err := NewFileLockerWithPollInterval(dir, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	locker1 := NewCompositeLocker(NewInMemoryLocker(), fileLocker1)
	locker2 := NewCompositeLocker(NewInMemoryLocker(), fileLocker2)

	unlock, err := locker1.LockSession(context.Background(), corepkg.SessionID("sess-1"))
	if err != nil {
		t.Fatalf("expected first composite lock to succeed, got %v", err)
	}
	defer func() {
		_ = unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = locker2.LockSession(ctx, corepkg.SessionID("sess-1"))
	if err == nil {
		t.Fatal("expected second composite lock to timeout")
	}
	if !hasErrorCode(err, ErrCodeLockTimeout) {
		t.Fatalf("expected lock timeout error code, got %v", err)
	}
}

func TestNewCompositeLockerHandlesNilSides(t *testing.T) {
	primary := NewInMemoryLocker()
	secondary := NewInMemoryLocker()

	if got := NewCompositeLocker(nil, secondary); got != secondary {
		t.Fatal("expected secondary when primary is nil")
	}
	if got := NewCompositeLocker(primary, nil); got != primary {
		t.Fatal("expected primary when secondary is nil")
	}
	if got := NewCompositeLocker(primary, secondary); got == nil {
		t.Fatal("expected composite locker when both are set")
	}
}

type compositeLockerStub struct {
	sessionUnlock UnlockFunc
	taskUnlock    UnlockFunc
	sessionErr    error
	taskErr       error
}

func (s *compositeLockerStub) LockSession(context.Context, corepkg.SessionID) (UnlockFunc, error) {
	if s.sessionErr != nil {
		return nil, s.sessionErr
	}
	if s.sessionUnlock != nil {
		return s.sessionUnlock, nil
	}
	return func() error { return nil }, nil
}

func (s *compositeLockerStub) LockTask(context.Context, corepkg.TaskID) (UnlockFunc, error) {
	if s.taskErr != nil {
		return nil, s.taskErr
	}
	if s.taskUnlock != nil {
		return s.taskUnlock, nil
	}
	return func() error { return nil }, nil
}

func TestCompositeLockerLockTaskSecondLayerFailureReleasesFirst(t *testing.T) {
	secondaryErr := errors.New("secondary lock failed")
	primaryUnlockErr := errors.New("primary unlock failed")

	locker := &CompositeLocker{
		primary: &compositeLockerStub{
			taskUnlock: func() error { return primaryUnlockErr },
		},
		secondary: &compositeLockerStub{taskErr: secondaryErr},
	}

	_, err := locker.LockTask(context.Background(), corepkg.TaskID("task-1"))
	if err == nil {
		t.Fatal("expected LockTask to fail when secondary lock fails")
	}
	if !errors.Is(err, secondaryErr) {
		t.Fatalf("expected secondary lock error, got %v", err)
	}
	if !errors.Is(err, primaryUnlockErr) {
		t.Fatalf("expected joined primary unlock error, got %v", err)
	}
}

func TestNewDefaultLockerProvidesTaskLocking(t *testing.T) {
	locker, err := NewDefaultLocker(t.TempDir())
	if err != nil {
		t.Fatalf("expected NewDefaultLocker to succeed, got %v", err)
	}
	firstUnlock, err := locker.LockTask(context.Background(), corepkg.TaskID("task-default"))
	if err != nil {
		t.Fatalf("expected first LockTask to succeed, got %v", err)
	}
	defer func() {
		_ = firstUnlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = locker.LockTask(ctx, corepkg.TaskID("task-default"))
	if err == nil {
		t.Fatal("expected second LockTask to timeout on same key")
	}
	if !hasErrorCode(err, ErrCodeLockTimeout) {
		t.Fatalf("expected lock timeout error code, got %v", err)
	}
}
