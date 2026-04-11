package runtime

import "context"

type TaskID string

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskKilled    TaskStatus = "killed"
)

// TaskScheduler controls task lifecycle.
type TaskScheduler interface {
	Submit(ctx context.Context, req SubmitRequest) (TaskHandle, error)
	Cancel(ctx context.Context, taskID TaskID) error
	ReadLogs(ctx context.Context, taskID TaskID, offset int64, limit int) ([]TaskLogChunk, error)
}

// TaskHandle represents an executable task instance.
type TaskHandle interface {
	ID() TaskID
	Wait(ctx context.Context) (TaskResult, error)
}

// SubmitRequest holds scheduling input.
type SubmitRequest struct {
	Name       string
	MaxRetries int
}

// TaskResult reports terminal task state.
type TaskResult struct {
	Status     TaskStatus
	ReasonCode string
}

// TaskLogChunk is one incremental log chunk with next offset cursor.
type TaskLogChunk struct {
	Offset int64
	Data   []byte
}
