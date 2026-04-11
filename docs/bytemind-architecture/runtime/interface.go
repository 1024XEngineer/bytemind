package runtime

import "context"

// TaskScheduler controls task lifecycle.
type TaskScheduler interface {
	Submit(ctx context.Context, req SubmitRequest) (TaskHandle, error)
	Cancel(ctx context.Context, taskID string) error
}

// TaskHandle represents an executable task instance.
type TaskHandle interface {
	ID() string
	Wait(ctx context.Context) (TaskResult, error)
}

// SubmitRequest holds scheduling input.
type SubmitRequest struct {
	Name string
}

// TaskResult reports terminal task state.
type TaskResult struct {
	Status string
}
