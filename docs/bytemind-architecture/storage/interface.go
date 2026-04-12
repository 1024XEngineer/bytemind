package storage

import (
	"context"
	"time"

	"bytemind/internal/core"
)

type ErrorCode string

const (
	ErrCodeNotFound        ErrorCode = "not_found"
	ErrCodeConflict        ErrorCode = "conflict"
	ErrCodeCorruptedRecord ErrorCode = "corrupted_record"
	ErrCodeWriteFailed     ErrorCode = "write_failed"
	ErrCodeReadFailed      ErrorCode = "read_failed"
	ErrCodeLockTimeout     ErrorCode = "lock_timeout"
)

type SessionEvent struct {
	SessionID core.SessionID
	EventID   string
	Offset    int64
	Type      string
	Payload   []byte
	CreatedAt time.Time
}

type TaskLogRecord struct {
	TaskID    core.TaskID
	EventID   string
	Offset    int64
	Level     string
	Message   string
	Payload   []byte
	CreatedAt time.Time
}

type SessionStore interface {
	Append(ctx context.Context, event SessionEvent) error
	ReadFrom(ctx context.Context, sessionID core.SessionID, offset int64, limit int) ([]SessionEvent, int64, error)
}

type TaskStore interface {
	AppendLog(ctx context.Context, record TaskLogRecord) error
	ReadLogFrom(ctx context.Context, taskID core.TaskID, offset int64, limit int) ([]TaskLogRecord, int64, error)
}

type Locker interface {
	LockSession(ctx context.Context, sessionID core.SessionID) (UnlockFunc, error)
	LockTask(ctx context.Context, taskID core.TaskID) (UnlockFunc, error)
}

type UnlockFunc func() error

type Deduplicator interface {
	Seen(ctx context.Context, stream string, eventID string) (bool, error)
	Mark(ctx context.Context, stream string, eventID string) error
}

type Replayer interface {
	ReplaySession(ctx context.Context, sessionID core.SessionID, fromOffset int64) (<-chan SessionEvent, error)
	ReplayTask(ctx context.Context, taskID core.TaskID, fromOffset int64) (<-chan TaskLogRecord, error)
}

