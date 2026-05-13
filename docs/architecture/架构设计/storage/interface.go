//go:build ignore

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
	Meta    core.EventMeta
	Offset  int64
	Type    string
	Payload []byte
}

type TaskLogRecord struct {
	Meta    core.EventMeta
	Offset  int64
	Level   string
	Message string
	Payload []byte
}

type AuditEvent struct {
	Meta       core.EventMeta
	Actor      string
	Action     string
	Decision   core.Decision
	ReasonCode string
	RiskLevel  core.RiskLevel
	Result     string
	LatencyMS  int64
	Payload    []byte
}

type SessionStore interface {
	Append(ctx context.Context, event SessionEvent) error
	ReadFrom(ctx context.Context, sessionID core.SessionID, offset int64, limit int) ([]SessionEvent, int64, error)
}

type TaskStore interface {
	AppendLog(ctx context.Context, record TaskLogRecord) error
	ReadLogFrom(ctx context.Context, taskID core.TaskID, offset int64, limit int) ([]TaskLogRecord, int64, error)
}

type AuditStore interface {
	Append(ctx context.Context, event AuditEvent) error
	ReadFrom(ctx context.Context, day time.Time, offset int64, limit int) ([]AuditEvent, int64, error)
}

type Locker interface {
	LockSession(ctx context.Context, sessionID core.SessionID) (UnlockFunc, error)
	LockTask(ctx context.Context, taskID core.TaskID) (UnlockFunc, error)
	LockAuditDay(ctx context.Context, day time.Time) (UnlockFunc, error)
}

type UnlockFunc func() error

type Deduplicator interface {
	Seen(ctx context.Context, stream string, eventID core.EventID) (bool, error)
	Mark(ctx context.Context, stream string, eventID core.EventID) error
}

type Replayer interface {
	ReplaySession(ctx context.Context, sessionID core.SessionID, fromOffset int64) (<-chan SessionEvent, error)
	ReplayTask(ctx context.Context, taskID core.TaskID, fromOffset int64) (<-chan TaskLogRecord, error)
	ReplayAudit(ctx context.Context, day time.Time, fromOffset int64) (<-chan AuditEvent, error)
}
