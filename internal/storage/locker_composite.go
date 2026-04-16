package storage

import (
	"context"
	"errors"
	"fmt"

	corepkg "bytemind/internal/core"
)

type CompositeLocker struct {
	primary   Locker
	secondary Locker
}

func NewCompositeLocker(primary, secondary Locker) Locker {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return &CompositeLocker{
		primary:   primary,
		secondary: secondary,
	}
}

func NewDefaultLocker(lockDir string) (Locker, error) {
	fileLocker, err := NewFileLocker(lockDir)
	if err != nil {
		return nil, err
	}
	return NewCompositeLocker(NewInMemoryLocker(), fileLocker), nil
}

func (l *CompositeLocker) LockSession(acquireCtx context.Context, sessionID corepkg.SessionID) (UnlockFunc, error) {
	if l == nil {
		return nil, errors.New("composite locker is nil")
	}
	firstUnlock, err := l.primary.LockSession(acquireCtx, sessionID)
	if err != nil {
		return nil, err
	}
	secondUnlock, err := l.secondary.LockSession(acquireCtx, sessionID)
	if err != nil {
		releaseErr := firstUnlock()
		if releaseErr != nil {
			return nil, errors.Join(err, fmt.Errorf("release primary session lock failed: %w", releaseErr))
		}
		return nil, err
	}
	return combineUnlocks(secondUnlock, firstUnlock), nil
}

func (l *CompositeLocker) LockTask(acquireCtx context.Context, taskID corepkg.TaskID) (UnlockFunc, error) {
	if l == nil {
		return nil, errors.New("composite locker is nil")
	}
	firstUnlock, err := l.primary.LockTask(acquireCtx, taskID)
	if err != nil {
		return nil, err
	}
	secondUnlock, err := l.secondary.LockTask(acquireCtx, taskID)
	if err != nil {
		releaseErr := firstUnlock()
		if releaseErr != nil {
			return nil, errors.Join(err, fmt.Errorf("release primary task lock failed: %w", releaseErr))
		}
		return nil, err
	}
	return combineUnlocks(secondUnlock, firstUnlock), nil
}

func combineUnlocks(unlocks ...UnlockFunc) UnlockFunc {
	return func() error {
		var combined error
		for _, unlock := range unlocks {
			if unlock == nil {
				continue
			}
			if err := unlock(); err != nil {
				combined = errors.Join(combined, err)
			}
		}
		return combined
	}
}
