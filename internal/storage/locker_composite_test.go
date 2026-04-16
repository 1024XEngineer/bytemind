package storage

import (
	"context"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
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
