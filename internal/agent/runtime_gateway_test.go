package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
	runtimepkg "bytemind/internal/runtime"
)

func TestDefaultRuntimeGatewayRunSyncCompletes(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager()
	gateway := newDefaultRuntimeGateway(manager)

	var (
		mu     sync.Mutex
		states []corepkg.TaskStatus
	)

	execution, err := gateway.RunSync(context.Background(), RuntimeTaskRequest{
		SessionID: "sess-1",
		TraceID:   "trace-1",
		Name:      "tool_success",
		Kind:      "tool",
		Execute: func(_ context.Context) ([]byte, error) {
			return []byte("ok"), nil
		},
		OnTaskStateChanged: func(task runtimepkg.Task) {
			mu.Lock()
			states = append(states, task.Status)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("RunSync failed: %v", err)
	}
	if execution.TaskID == "" {
		t.Fatal("expected non-empty task id")
	}
	if execution.Result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected completed status, got %s", execution.Result.Status)
	}
	if got := string(execution.Result.Output); got != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(states) < 3 {
		t.Fatalf("expected at least 3 state callbacks, got %v", states)
	}
	if states[0] != corepkg.TaskPending && states[0] != corepkg.TaskRunning {
		t.Fatalf("expected first callback to be pending or running, got %v", states)
	}
	if states[len(states)-1] != corepkg.TaskCompleted {
		t.Fatalf("expected final callback completed, got %v", states)
	}
	if !containsTaskStatus(states, corepkg.TaskRunning) {
		t.Fatalf("expected running status callback, got %v", states)
	}
	if err := assertMonotonicTaskProgress(states); err != nil {
		t.Fatalf("expected monotonic state progression, got states=%v err=%v", states, err)
	}
}

func TestDefaultRuntimeGatewayRunSyncCapturesExecutionFailure(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager()
	gateway := newDefaultRuntimeGateway(manager)

	execution, err := gateway.RunSync(context.Background(), RuntimeTaskRequest{
		SessionID: "sess-1",
		TraceID:   "trace-fail",
		Name:      "tool_fail",
		Kind:      "tool",
		Execute: func(_ context.Context) ([]byte, error) {
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("RunSync failed: %v", err)
	}
	if execution.Result.Status != corepkg.TaskFailed {
		t.Fatalf("expected failed status, got %s", execution.Result.Status)
	}
	if execution.Result.ErrorCode != runtimepkg.ErrorCodeTaskExecutionFailed {
		t.Fatalf("expected error code %q, got %q", runtimepkg.ErrorCodeTaskExecutionFailed, execution.Result.ErrorCode)
	}
	if execution.ExecutionError == nil || execution.ExecutionError.Error() != "boom" {
		t.Fatalf("expected execution error boom, got %v", execution.ExecutionError)
	}
}

func TestDefaultRuntimeGatewayRunSyncCancelsTaskWhenParentContextCancelled(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager()
	gateway := newDefaultRuntimeGateway(manager)

	started := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan RuntimeTaskExecution, 1)
	errCh := make(chan error, 1)
	go func() {
		execution, err := gateway.RunSync(ctx, RuntimeTaskRequest{
			SessionID: "sess-1",
			TraceID:   "trace-cancel",
			Name:      "tool_cancel",
			Kind:      "tool",
			Execute: func(execCtx context.Context) ([]byte, error) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-execCtx.Done()
				return nil, execCtx.Err()
			},
		})
		resultCh <- execution
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start")
	}
	cancel()

	select {
	case execution := <-resultCh:
		err := <-errCh
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled error, got %v", err)
		}
		if execution.TaskID == "" {
			t.Fatal("expected task id when cancelled")
		}
		if execution.Result.Status != corepkg.TaskKilled {
			t.Fatalf("expected killed status, got %s", execution.Result.Status)
		}
		if execution.Result.ErrorCode != runtimepkg.ErrorCodeTaskCancelled {
			t.Fatalf("expected cancel error code %q, got %q", runtimepkg.ErrorCodeTaskCancelled, execution.Result.ErrorCode)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("RunSync did not return after parent cancellation")
	}
}

func TestDefaultRuntimeGatewayRunSyncFailsWhenRegistryUnavailable(t *testing.T) {
	gateway := newDefaultRuntimeGateway(taskManagerWithoutRegistry{})

	_, err := gateway.RunSync(context.Background(), RuntimeTaskRequest{
		SessionID: "sess-1",
		TraceID:   "trace-no-registry",
		Name:      "tool_no_registry",
		Kind:      "tool",
		Execute: func(_ context.Context) ([]byte, error) {
			return []byte("ok"), nil
		},
	})
	if err == nil {
		t.Fatal("expected RunSync to fail when registry is unavailable")
	}
	if got := err.Error(); got != "runtime task manager must implement TaskExecutionRegistry" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsTaskStatus(states []corepkg.TaskStatus, expected corepkg.TaskStatus) bool {
	for _, status := range states {
		if status == expected {
			return true
		}
	}
	return false
}

func assertMonotonicTaskProgress(states []corepkg.TaskStatus) error {
	if len(states) == 0 {
		return fmt.Errorf("empty states")
	}
	previousRank := -1
	for _, status := range states {
		rank, ok := taskStatusRank(status)
		if !ok {
			return fmt.Errorf("unknown status %q", status)
		}
		if previousRank > rank {
			return fmt.Errorf("status regressed from rank %d to %d", previousRank, rank)
		}
		previousRank = rank
	}
	last := states[len(states)-1]
	if !isTerminalStatus(last) {
		return fmt.Errorf("final status %q is not terminal", last)
	}
	return nil
}

func taskStatusRank(status corepkg.TaskStatus) (int, bool) {
	switch status {
	case corepkg.TaskPending:
		return 0, true
	case corepkg.TaskRunning:
		return 1, true
	case corepkg.TaskCompleted, corepkg.TaskFailed, corepkg.TaskKilled:
		return 2, true
	default:
		return 0, false
	}
}

func isTerminalStatus(status corepkg.TaskStatus) bool {
	return status == corepkg.TaskCompleted || status == corepkg.TaskFailed || status == corepkg.TaskKilled
}

type taskManagerWithoutRegistry struct{}

func (taskManagerWithoutRegistry) Submit(_ context.Context, _ runtimepkg.TaskSpec) (corepkg.TaskID, error) {
	return "task-id", nil
}

func (taskManagerWithoutRegistry) Get(_ context.Context, _ corepkg.TaskID) (runtimepkg.Task, error) {
	return runtimepkg.Task{}, nil
}

func (taskManagerWithoutRegistry) Cancel(_ context.Context, _ corepkg.TaskID, _ string) error {
	return nil
}

func (taskManagerWithoutRegistry) Retry(_ context.Context, _ corepkg.TaskID) (corepkg.TaskID, error) {
	return "", nil
}

func (taskManagerWithoutRegistry) Stream(_ context.Context, _ corepkg.TaskID) (<-chan runtimepkg.TaskEvent, error) {
	return nil, nil
}

func (taskManagerWithoutRegistry) Wait(_ context.Context, _ corepkg.TaskID) (runtimepkg.TaskResult, error) {
	return runtimepkg.TaskResult{}, nil
}
