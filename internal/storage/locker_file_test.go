package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
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

func TestFileLockerReclaimsStaleLockFileByMtime(t *testing.T) {
	dir := t.TempDir()
	locker, err := NewFileLockerWithConfig(dir, 5*time.Millisecond, 20*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	key := "session:stale"
	lockPath := locker.lockPath(key)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("pid=999999\nkey=session:stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	unlock, err := locker.LockSession(context.Background(), corepkg.SessionID("stale"))
	if err != nil {
		t.Fatalf("expected lock acquisition after stale reclaim, got %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("expected unlock to succeed, got %v", err)
	}
}

func TestNewFileLockerWithConfigValidationAndDefaults(t *testing.T) {
	if _, err := NewFileLockerWithConfig("   ", 0, 0, 0); err == nil {
		t.Fatal("expected blank dir to fail")
	}

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileLockerWithConfig(filePath, 0, 0, 0); err == nil {
		t.Fatal("expected file path as dir to fail")
	}

	locker, err := NewFileLockerWithConfig(filepath.Join(tmp, "locks"), 0, 0, 0)
	if err != nil {
		t.Fatalf("expected defaults to be applied, got %v", err)
	}
	if locker.pollInterval <= 0 || locker.staleAfter <= 0 || locker.heartbeatInterval <= 0 {
		t.Fatal("expected positive default intervals")
	}
}

func TestFileLockerTryReclaimStaleBranches(t *testing.T) {
	dir := t.TempDir()
	locker, err := NewFileLockerWithConfig(dir, 5*time.Millisecond, 50*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	missingPath := filepath.Join(dir, "missing.lock")
	reclaimed, err := locker.tryReclaimStaleLock(missingPath)
	if err != nil {
		t.Fatalf("expected missing lock reclaim to succeed, got %v", err)
	}
	if !reclaimed {
		t.Fatal("expected missing lock to be treated as reclaimed")
	}

	freshPath := filepath.Join(dir, "fresh.lock")
	if err := os.WriteFile(freshPath, []byte("pid=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := os.Chtimes(freshPath, now, now); err != nil {
		t.Fatal(err)
	}
	reclaimed, err = locker.tryReclaimStaleLock(freshPath)
	if err != nil {
		t.Fatalf("expected fresh lock check to succeed, got %v", err)
	}
	if reclaimed {
		t.Fatal("expected fresh lock not to be reclaimed")
	}

	old := now.Add(-time.Hour)
	if err := os.Chtimes(freshPath, old, old); err != nil {
		t.Fatal(err)
	}
	reclaimed, err = locker.tryReclaimStaleLock(freshPath)
	if err != nil {
		t.Fatalf("expected stale lock reclaim to succeed, got %v", err)
	}
	if !reclaimed {
		t.Fatal("expected stale lock to be reclaimed")
	}
	if _, statErr := os.Stat(freshPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected stale lock file removed, got err=%v", statErr)
	}
}

func TestFileLockerRejectsEmptyIDs(t *testing.T) {
	locker, err := NewFileLocker(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := locker.LockSession(context.Background(), corepkg.SessionID("   ")); err == nil {
		t.Fatal("expected empty session id to fail")
	}
	if _, err := locker.LockTask(context.Background(), corepkg.TaskID("")); err == nil {
		t.Fatal("expected empty task id to fail")
	}
}

func TestFileLockerNilReceiver(t *testing.T) {
	var locker *FileLocker
	if _, err := locker.lock(context.Background(), "session:any"); err == nil {
		t.Fatal("expected nil locker to fail")
	}
}
