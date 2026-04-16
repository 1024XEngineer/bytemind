package storage

import (
	"context"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestFileLockerSameKeyContentionTimeout(t *testing.T) {
	dir := t.TempDir()
	locker1, err := NewFileLockerWithPollInterval(dir, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	locker2, err := NewFileLockerWithPollInterval(dir, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	heldUnlock, err := locker1.LockSession(context.Background(), corepkg.SessionID("sess-1"))
	if err != nil {
		t.Fatalf("expected first lock to succeed, got %v", err)
	}
	defer func() {
		_ = heldUnlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = locker2.LockSession(ctx, corepkg.SessionID("sess-1"))
	if err == nil {
		t.Fatal("expected second lock to timeout")
	}
	if !hasErrorCode(err, ErrCodeLockTimeout) {
		t.Fatalf("expected lock timeout error code, got %v", err)
	}
}

func TestFileLockerReleaseAllowsOtherInstance(t *testing.T) {
	dir := t.TempDir()
	locker1, err := NewFileLocker(dir)
	if err != nil {
		t.Fatal(err)
	}
	locker2, err := NewFileLocker(dir)
	if err != nil {
		t.Fatal(err)
	}

	firstUnlock, err := locker1.LockTask(context.Background(), corepkg.TaskID("task-1"))
	if err != nil {
		t.Fatalf("expected first lock to succeed, got %v", err)
	}
	if err := firstUnlock(); err != nil {
		t.Fatalf("expected first unlock to succeed, got %v", err)
	}

	secondUnlock, err := locker2.LockTask(context.Background(), corepkg.TaskID("task-1"))
	if err != nil {
		t.Fatalf("expected second lock to succeed after release, got %v", err)
	}
	if err := secondUnlock(); err != nil {
		t.Fatalf("expected second unlock to succeed, got %v", err)
	}
}

func TestFileLockerDifferentKeysDoNotConflict(t *testing.T) {
	dir := t.TempDir()
	locker, err := NewFileLocker(dir)
	if err != nil {
		t.Fatal(err)
	}

	sessionUnlock, err := locker.LockSession(context.Background(), corepkg.SessionID("shared"))
	if err != nil {
		t.Fatalf("expected session lock to succeed, got %v", err)
	}
	defer func() {
		_ = sessionUnlock()
	}()

	taskUnlock, err := locker.LockTask(context.Background(), corepkg.TaskID("shared"))
	if err != nil {
		t.Fatalf("expected task lock with same id to succeed, got %v", err)
	}
	if err := taskUnlock(); err != nil {
		t.Fatalf("expected task unlock to succeed, got %v", err)
	}
}
