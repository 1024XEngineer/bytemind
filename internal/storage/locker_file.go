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

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

const (
	defaultFileLockPollInterval = 20 * time.Millisecond
	defaultFileLockStaleAfter   = 2 * time.Minute
	defaultFileLockHeartbeat    = 10 * time.Second
)

type FileLocker struct {
	dir               string
	pollInterval      time.Duration
	staleAfter        time.Duration
	heartbeatInterval time.Duration
	now               func() time.Time
}

func NewFileLocker(dir string) (*FileLocker, error) {
	return NewFileLockerWithConfig(
		dir,
		defaultFileLockPollInterval,
		defaultFileLockStaleAfter,
		defaultFileLockHeartbeat,
	)
}

func NewFileLockerWithPollInterval(dir string, pollInterval time.Duration) (*FileLocker, error) {
	return NewFileLockerWithConfig(
		dir,
		pollInterval,
		defaultFileLockStaleAfter,
		defaultFileLockHeartbeat,
	)
}

func NewFileLockerWithConfig(dir string, pollInterval, staleAfter, heartbeatInterval time.Duration) (*FileLocker, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("file locker dir is required")
	}
	if pollInterval <= 0 {
		pollInterval = defaultFileLockPollInterval
	}
	if staleAfter <= 0 {
		staleAfter = defaultFileLockStaleAfter
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultFileLockHeartbeat
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileLocker{
		dir:               dir,
		pollInterval:      pollInterval,
		staleAfter:        staleAfter,
		heartbeatInterval: heartbeatInterval,
		now: func() time.Time {
			return time.Now().UTC()
		},
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
			now := l.now().UTC()
			if _, writeErr := fmt.Fprintf(file, "pid=%d\nts=%s\nkey=%s\n", os.Getpid(), now.Format(time.RFC3339Nano), key); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return nil, closeErr
			}
			_ = os.Chtimes(path, now, now)

			stopHeartbeat := make(chan struct{})
			heartbeatDone := make(chan struct{})
			go l.heartbeat(path, stopHeartbeat, heartbeatDone)

			var released atomic.Bool
			return func() error {
				if !released.CompareAndSwap(false, true) {
					return nil
				}
				close(stopHeartbeat)
				<-heartbeatDone
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				return nil
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		reclaimed, reclaimErr := l.tryReclaimStaleLock(path)
		if reclaimErr != nil {
			return nil, reclaimErr
		}
		if reclaimed {
			continue
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

func (l *FileLocker) heartbeat(path string, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(l.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := l.now().UTC()
			_ = os.Chtimes(path, now, now)
		}
	}
}

func (l *FileLocker) tryReclaimStaleLock(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	if !l.isStaleLock(info) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

func (l *FileLocker) isStaleLock(info os.FileInfo) bool {
	if l.staleAfter > 0 && l.now().Sub(info.ModTime()) >= l.staleAfter {
		return true
	}
	return false
}

func (l *FileLocker) lockPath(key string) string {
	sum := sha1.Sum([]byte(key))
	base := hex.EncodeToString(sum[:])
	return filepath.Join(l.dir, base+".lock")
}
