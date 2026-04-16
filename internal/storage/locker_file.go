package storage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	corepkg "bytemind/internal/core"
)

const defaultFileLockPollInterval = 20 * time.Millisecond

type FileLocker struct {
	dir          string
	pollInterval time.Duration
}

func NewFileLocker(dir string) (*FileLocker, error) {
	return NewFileLockerWithPollInterval(dir, defaultFileLockPollInterval)
}

func NewFileLockerWithPollInterval(dir string, pollInterval time.Duration) (*FileLocker, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("file locker dir is required")
	}
	if pollInterval <= 0 {
		pollInterval = defaultFileLockPollInterval
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileLocker{
		dir:          dir,
		pollInterval: pollInterval,
	}, nil
}

func (l *FileLocker) LockSession(acquireCtx context.Context, sessionID corepkg.SessionID) (UnlockFunc, error) {
	id := strings.TrimSpace(string(sessionID))
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return l.lock(acquireCtx, "session:"+id)
}

func (l *FileLocker) LockTask(acquireCtx context.Context, taskID corepkg.TaskID) (UnlockFunc, error) {
	id := strings.TrimSpace(string(taskID))
	if id == "" {
		return nil, fmt.Errorf("task id is required")
	}
	return l.lock(acquireCtx, "task:"+id)
}

func (l *FileLocker) lock(acquireCtx context.Context, key string) (UnlockFunc, error) {
	if l == nil {
		return nil, errors.New("file locker is nil")
	}
	if acquireCtx == nil {
		acquireCtx = context.Background()
	}

	path := l.lockPath(key)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			if _, writeErr := fmt.Fprintf(file, "pid=%d\nts=%s\nkey=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano), key); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, closeErr
			}

			var released atomic.Bool
			return func() error {
				if !released.CompareAndSwap(false, true) {
					return nil
				}
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				return nil
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		select {
		case <-acquireCtx.Done():
			if errors.Is(acquireCtx.Err(), context.DeadlineExceeded) {
				return nil, newLockerError(
					ErrCodeLockTimeout,
					fmt.Sprintf("file lock %q acquisition timed out", key),
					true,
					acquireCtx.Err(),
				)
			}
			return nil, acquireCtx.Err()
		case <-time.After(l.pollInterval):
		}
	}
}

func (l *FileLocker) lockPath(key string) string {
	sum := sha1.Sum([]byte(key))
	base := hex.EncodeToString(sum[:])
	return filepath.Join(l.dir, base+".lock")
}
