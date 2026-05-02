package storage

import (
	"context"
	"testing"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestInMemoryLockerSameKeyTimeout(t *testing.T) {
	locker := NewInMemoryLocker()

	firstCtx, firstCancel := context.WithTimeout(context.Background(), time.Second)
	defer firstCancel()
	unlock, err := locker.LockSession(firstCtx, corepkg.SessionID("sess-1"))
	if err != nil {
		t.Fatalf("expected first lock to succeed, got %v", err)
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil {
			t.Fatalf("unlock failed: %v", unlockErr)
		}
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer waitCancel()
	_, err = locker.LockSession(waitCtx, corepkg.SessionID("sess-1"))
	if err == nil {
		t.Fatal("expected lock timeout error for same session key")
	}
	if !hasErrorCode(err, ErrCodeLockTimeout) {
		t.Fatalf("expected error code %q, got %v", ErrCodeLockTimeout, err)
	}
}

func TestInMemoryLockerReleaseAllowsReentry(t *testing.T) {
	locker := NewInMemoryLocker()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	firstUnlock, err := locker.LockSession(ctx, corepkg.SessionID("sess-1"))
	if err != nil {
		t.Fatalf("expected first lock to succeed, got %v", err)
	}
	if err := firstUnlock(); err != nil {
		t.Fatalf("expected unlock to succeed, got %v", err)
	}
	if err := firstUnlock(); err != nil {
		t.Fatalf("expected repeated unlock to be idempotent, got %v", err)
	}

	secondUnlock, err := locker.LockSession(ctx, corepkg.SessionID("sess-1"))
	if err != nil {
		t.Fatalf("expected re-entry lock to succeed, got %v", err)
	}
	if err := secondUnlock(); err != nil {
		t.Fatalf("expected second unlock to succeed, got %v", err)
	}
}

func TestInMemoryLockerDifferentScopesDoNotBlock(t *testing.T) {
	locker := NewInMemoryLocker()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sessionUnlock, err := locker.LockSession(ctx, corepkg.SessionID("shared"))
	if err != nil {
		t.Fatalf("expected session lock to succeed, got %v", err)
	}
	defer func() {
		if unlockErr := sessionUnlock(); unlockErr != nil {
			t.Fatalf("expected session unlock to succeed, got %v", unlockErr)
		}
	}()

	otherSessionUnlock, err := locker.LockSession(ctx, corepkg.SessionID("other"))
	if err != nil {
		t.Fatalf("expected different session key lock to succeed, got %v", err)
	}
	if err := otherSessionUnlock(); err != nil {
		t.Fatalf("expected unlock to succeed, got %v", err)
	}

	taskUnlock, err := locker.LockTask(ctx, corepkg.TaskID("shared"))
	if err != nil {
		t.Fatalf("expected task lock with shared id to succeed, got %v", err)
	}
	if err := taskUnlock(); err != nil {
		t.Fatalf("expected task unlock to succeed, got %v", err)
	}
}
