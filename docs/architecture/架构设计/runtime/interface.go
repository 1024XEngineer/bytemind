//go:build ignore

package runtime

import (
	"context"
	"time"

	"bytemind/internal/core"
)

type ErrorCode string

const (
	ErrCodeInvalidTransition ErrorCode = "invalid_transition"
	ErrCodeTaskNotFound      ErrorCode = "task_not_found"
	ErrCodeTaskTimeout       ErrorCode = "task_timeout"
	ErrCodeTaskCanceled      ErrorCode = "task_canceled"
	ErrCodeRetryExhausted    ErrorCode = "retry_exhausted"
	ErrCodeQuotaExceeded     ErrorCode = "quota_exceeded"
)

type TaskSpec struct {
	SessionID        core.SessionID
	TraceID          core.TraceID
	Name             string
	Kind             string
	Input            []byte
	ParentTaskID     core.TaskID
	Timeout          time.Duration
	MaxRetries       int
	Background       bool
	IsolatedWorktree bool
	Metadata         map[string]string
}

type Task struct {
	ID         core.TaskID
	Spec       TaskSpec
	Status     core.TaskStatus
	Attempt    int
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	ErrorCode  string
}

type TaskResult struct {
	TaskID     core.TaskID
	Status     core.TaskStatus
	Output     []byte
	ErrorCode  string
	FinishedAt time.Time
}

type TaskLogEntry struct {
	TaskID  core.TaskID
	Offset  int64
	Level   string
	Message string
	Meta    core.EventMeta
}

type TaskEventType string

const (
	TaskEventStatus TaskEventType = "status"
	TaskEventLog    TaskEventType = "log"
	TaskEventResult TaskEventType = "result"
	TaskEventError  TaskEventType = "error"
)

type TaskEvent struct {
	Type      TaskEventType
	TaskID    core.TaskID
	Status    core.TaskStatus
	Log       *TaskLogEntry
	Result    *TaskResult
	Meta      core.EventMeta
	ErrorCode string
}

type TaskManager interface {
	Submit(ctx context.Context, spec TaskSpec) (core.TaskID, error)
	Get(ctx context.Context, id core.TaskID) (Task, error)
	Cancel(ctx context.Context, id core.TaskID, reason string) error
	Retry(ctx context.Context, id core.TaskID) (core.TaskID, error)
	Wait(ctx context.Context, id core.TaskID) (TaskResult, error)
	Stream(ctx context.Context, id core.TaskID) (<-chan TaskEvent, error)
}

type Scheduler interface {
	Enqueue(ctx context.Context, id core.TaskID) error
	Pause(ctx context.Context, id core.TaskID) error
	Resume(ctx context.Context, id core.TaskID) error
}

type LogReader interface {
	ReadIncrement(ctx context.Context, id core.TaskID, fromOffset int64, limit int) ([]TaskLogEntry, int64, error)
}

type SubAgentCoordinator interface {
	Spawn(ctx context.Context, parent core.TaskID, spec TaskSpec) (core.TaskID, error)
	Wait(ctx context.Context, id core.TaskID) (TaskResult, error)
	CollectBackground(ctx context.Context, parent core.TaskID) ([]TaskResult, error)
}

type QuotaManager interface {
	Acquire(ctx context.Context, key string, n int) error
	Release(ctx context.Context, key string, n int) error
}
