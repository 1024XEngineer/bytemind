package session

import (
	"context"
	"errors"
	"testing"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

type storeLockerStub struct {
	lockErr          error
	unlockErr        error
	lockSessionCalls int
	unlockCalls      int
}

func (s *storeLockerStub) LockSession(_ context.Context, _ corepkg.SessionID) (storagepkg.UnlockFunc, error) {
	s.lockSessionCalls++
	if s.lockErr != nil {
		return nil, s.lockErr
	}
	return func() error {
		s.unlockCalls++
		return s.unlockErr
	}, nil
}

func (s *storeLockerStub) LockTask(context.Context, corepkg.TaskID) (storagepkg.UnlockFunc, error) {
	return func() error { return nil }, nil
}

func TestStoreSaveUsesSessionLocker(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	locker := &storeLockerStub{}
	store.locker = locker

	sess := New(t.TempDir())
	sess.ID = "locker-save"
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if locker.lockSessionCalls != 1 {
		t.Fatalf("expected LockSession to be called once, got %d", locker.lockSessionCalls)
	}
	if locker.unlockCalls != 1 {
		t.Fatalf("expected unlock to be called once, got %d", locker.unlockCalls)
	}
}

func TestStoreSaveReturnsLockerError(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	lockErr := errors.New("lock timeout")
	store.locker = &storeLockerStub{lockErr: lockErr}

	sess := New(t.TempDir())
	sess.ID = "locker-error"
	err = store.Save(sess)
	if err == nil {
		t.Fatal("expected Save to fail when locker acquisition fails")
	}
	if !errors.Is(err, lockErr) {
		t.Fatalf("expected Save to return locker error, got %v", err)
	}
}

func TestStoreSaveReturnsUnlockError(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	unlockErr := errors.New("unlock failed")
	store.locker = &storeLockerStub{unlockErr: unlockErr}

	sess := New(t.TempDir())
	sess.ID = "locker-unlock-error"
	err = store.Save(sess)
	if err == nil {
		t.Fatal("expected Save to fail when unlock fails")
	}
	if !errors.Is(err, unlockErr) {
		t.Fatalf("expected Save to include unlock error, got %v", err)
	}
}
