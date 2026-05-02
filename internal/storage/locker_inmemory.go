package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type inMemoryLockEntry struct {
	sem      chan struct{}
	refCount int64
}

type InMemoryLocker struct {
	mu    sync.Mutex
	locks map[string]*inMemoryLockEntry
}

func NewInMemoryLocker() *InMemoryLocker {
	return &InMemoryLocker{
		locks: make(map[string]*inMemoryLockEntry),
	}
}

func (l *InMemoryLocker) LockSession(ctx context.Context, sessionID corepkg.SessionID) (UnlockFunc, error) {
	id := strings.TrimSpace(string(sessionID))
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return l.lock(ctx, "session:"+id)
}

func (l *InMemoryLocker) LockTask(ctx context.Context, taskID corepkg.TaskID) (UnlockFunc, error) {
	id := strings.TrimSpace(string(taskID))
	if id == "" {
		return nil, fmt.Errorf("task id is required")
	}
	return l.lock(ctx, "task:"+id)
}

func (l *InMemoryLocker) lock(ctx context.Context, key string) (UnlockFunc, error) {
	if l == nil {
		return nil, fmt.Errorf("locker is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	entry := l.acquireEntry(key)
	select {
	case entry.sem <- struct{}{}:
		var released atomic.Bool
		return func() error {
			if !released.CompareAndSwap(false, true) {
				return nil
			}
			select {
			case <-entry.sem:
			default:
			}
			l.releaseEntry(key)
			return nil
		}, nil

	case <-ctx.Done():
		l.releaseEntry(key)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, newLockerError(
				ErrCodeLockTimeout,
				fmt.Sprintf("lock %q acquisition timed out", key),
				true,
				ctx.Err(),
			)
		}
		return nil, ctx.Err()
	}
}

func (l *InMemoryLocker) acquireEntry(key string) *inMemoryLockEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.locks[key]
	if entry == nil {
		entry = &inMemoryLockEntry{sem: make(chan struct{}, 1)}
		l.locks[key] = entry
	}
	entry.refCount++
	return entry
}

func (l *InMemoryLocker) releaseEntry(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.locks[key]
	if entry == nil {
		return
	}
	entry.refCount--
	if entry.refCount <= 0 && len(entry.sem) == 0 {
		delete(l.locks, key)
	}
}
