package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestInMemoryTaskManagerSubmitAndCancel(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected task id")
	}
	if err := mgr.Cancel(context.Background(), id, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	task, err := mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != "killed" {
		t.Fatalf("expected killed status, got %s", task.Status)
	}
	if task.ErrorCode != ErrorCodeTaskCancelled {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskCancelled, task.ErrorCode)
	}
}

func TestInMemoryTaskManagerWaitReturnsTerminalResult(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	results := make(chan TaskResult, 1)
	waitErrs := make(chan error, 1)
	go func() {
		result, err := mgr.Wait(context.Background(), id)
		if err != nil {
			waitErrs <- err
			return
		}
		results <- result
	}()

	time.Sleep(10 * time.Millisecond)
	if err := mgr.Cancel(context.Background(), id, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	select {
	case err := <-waitErrs:
		t.Fatalf("Wait failed: %v", err)
	case result := <-results:
		if result.TaskID != id {
			t.Fatalf("expected task id %q, got %q", id, result.TaskID)
		}
		if result.Status != "killed" {
			t.Fatalf("expected killed status, got %s", result.Status)
		}
		if result.ErrorCode != ErrorCodeTaskCancelled {
			t.Fatalf("expected error code %q, got %q", ErrorCodeTaskCancelled, result.ErrorCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait timed out")
	}
}

func TestInMemoryTaskManagerWaitRespectsContextCancellation(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = mgr.Wait(ctx, id)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestInMemoryTaskManagerGetUnknownTaskReturnsTaskNotFound(t *testing.T) {
	mgr := NewInMemoryTaskManager()

	_, err := mgr.Get(context.Background(), "unknown-task")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	if !hasErrorCode(err, ErrorCodeTaskNotFound) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskNotFound, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryFromFailedResetsTaskForRetry(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:       "demo",
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	finishedAt := time.Now().UTC().Add(-1 * time.Second)
	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskFailed
	task.Attempt = 1
	task.StartedAt = &startedAt
	task.FinishedAt = &finishedAt
	task.ErrorCode = ErrorCodeTaskTimeout
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	retriedID, err := mgr.Retry(context.Background(), id)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}
	if retriedID != id {
		t.Fatalf("expected retried id %q, got %q", id, retriedID)
	}

	task, err = mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != corepkg.TaskPending {
		t.Fatalf("expected pending status, got %s", task.Status)
	}
	if task.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", task.Attempt)
	}
	if task.ErrorCode != "" {
		t.Fatalf("expected cleared error code, got %q", task.ErrorCode)
	}
	if task.StartedAt != nil {
		t.Fatal("expected startedAt to reset on retry")
	}
	if task.FinishedAt != nil {
		t.Fatal("expected finishedAt to reset on retry")
	}
}

func TestInMemoryTaskManagerRetryRejectsNonFailedTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	_, err = mgr.Retry(context.Background(), id)
	if err == nil {
		t.Fatal("expected retry error for non-failed task")
	}
	if !hasErrorCode(err, ErrorCodeInvalidTransition) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeInvalidTransition, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryExhausted(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:       "demo",
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskFailed
	task.Attempt = 1
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	_, err = mgr.Retry(context.Background(), id)
	if err == nil {
		t.Fatal("expected retry exhausted error")
	}
	if !hasErrorCode(err, ErrorCodeRetryExhausted) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeRetryExhausted, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryUnknownTaskReturnsTaskNotFound(t *testing.T) {
	mgr := NewInMemoryTaskManager()

	_, err := mgr.Retry(context.Background(), "missing-task")
	if err == nil {
		t.Fatal("expected retry error for unknown task")
	}
	if !hasErrorCode(err, ErrorCodeTaskNotFound) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskNotFound, errorCode(err))
	}
}

func TestInMemoryTaskManagerCancelIsIdempotent(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if err := mgr.Cancel(context.Background(), id, "first cancel"); err != nil {
		t.Fatalf("first cancel failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "second cancel"); err != nil {
		t.Fatalf("second cancel should be idempotent, got: %v", err)
	}
}

func TestInMemoryTaskManagerCancelRejectsCompletedTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	finishedAt := time.Now().UTC()
	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskCompleted
	task.FinishedAt = &finishedAt
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	err = mgr.Cancel(context.Background(), id, "cancel completed")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if !hasErrorCode(err, ErrorCodeInvalidTransition) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeInvalidTransition, errorCode(err))
	}
}

func TestInMemoryTaskManagerWaitReturnsImmediatelyForTerminalTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "terminal"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := mgr.Wait(ctx, id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskKilled {
		t.Fatalf("expected killed status, got %s", result.Status)
	}
}

func TestInMemoryTaskManagerWaitWithNilContextUsesBackground(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "terminal"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	result, err := mgr.Wait(nil, id)
	if err != nil {
		t.Fatalf("Wait with nil context failed: %v", err)
	}
	if result.TaskID != id {
		t.Fatalf("expected task id %q, got %q", id, result.TaskID)
	}
}

func TestInMemoryTaskManagerStreamReturnsNotImplemented(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	_, err := mgr.Stream(context.Background(), "task-id")
	if err == nil {
		t.Fatal("expected stream to return not implemented")
	}
	if !hasErrorCode(err, ErrorCodeNotImplemented) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeNotImplemented, errorCode(err))
	}
}
