package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
	runtimepkg "bytemind/internal/runtime"
)

func TestTaskStopToolStopsRunningTask(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager(runtimepkg.WithTaskExecutor(func(ctx context.Context, _ runtimepkg.Task) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	taskID, err := manager.Submit(context.Background(), runtimepkg.TaskSpec{
		SessionID: "sess-stop",
		Name:      "subagent",
		Kind:      "subagent",
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	waitForTaskStatus(t, manager, taskID, corepkg.TaskRunning, 2*time.Second)

	tool := TaskStopTool{}
	out, runErr := tool.Run(context.Background(), []byte(`{"task_id":"`+string(taskID)+`","reason":"user_stop"}`), &ExecutionContext{
		TaskManager: manager,
	})
	if runErr != nil {
		t.Fatalf("task_stop run failed: %v", runErr)
	}

	var result struct {
		OK         bool   `json:"ok"`
		TaskID     string `json:"task_id"`
		Status     string `json:"status"`
		TaskStatus string `json:"task_status"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal task_stop result: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result.TaskID != string(taskID) {
		t.Fatalf("expected task id %q, got %q", taskID, result.TaskID)
	}
	if result.Status != "stop_requested" && result.Status != "already_terminal" {
		t.Fatalf("unexpected stop status: %#v", result)
	}

	final, waitErr := manager.Wait(context.Background(), taskID)
	if waitErr != nil {
		t.Fatalf("wait stopped task failed: %v", waitErr)
	}
	if final.Status != corepkg.TaskKilled {
		t.Fatalf("expected killed status, got %s", final.Status)
	}
}

func TestTaskStopToolReturnsAlreadyTerminalForCompletedTask(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager(runtimepkg.WithTaskExecutor(func(_ context.Context, _ runtimepkg.Task) ([]byte, error) {
		return []byte("ok"), nil
	}))
	taskID, err := manager.Submit(context.Background(), runtimepkg.TaskSpec{
		SessionID: "sess-terminal",
		Name:      "subagent",
		Kind:      "subagent",
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if _, err := manager.Wait(context.Background(), taskID); err != nil {
		t.Fatalf("wait task: %v", err)
	}

	tool := TaskStopTool{}
	out, runErr := tool.Run(context.Background(), []byte(`{"task_id":"`+string(taskID)+`"}`), &ExecutionContext{
		TaskManager: manager,
	})
	if runErr != nil {
		t.Fatalf("task_stop run failed: %v", runErr)
	}

	var result struct {
		OK         bool   `json:"ok"`
		Status     string `json:"status"`
		TaskStatus string `json:"task_status"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal task_stop result: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result.Status != "already_terminal" {
		t.Fatalf("expected already_terminal status, got %#v", result)
	}
	if result.TaskStatus != string(corepkg.TaskCompleted) {
		t.Fatalf("expected completed task status, got %q", result.TaskStatus)
	}
}

func TestTaskStopToolReturnsInvalidArgsForUnknownTask(t *testing.T) {
	tool := TaskStopTool{}
	_, err := tool.Run(context.Background(), []byte(`{"task_id":"missing-task"}`), &ExecutionContext{
		TaskManager: runtimepkg.NewInMemoryTaskManager(),
	})
	if err == nil {
		t.Fatal("expected unknown task error")
	}
	execErr, ok := err.(*ToolExecError)
	if !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("expected invalid args error code, got %T %#v", err, err)
	}
}

func TestTaskStopToolRequiresTaskManager(t *testing.T) {
	tool := TaskStopTool{}
	_, err := tool.Run(context.Background(), []byte(`{"task_id":"task-1"}`), &ExecutionContext{})
	if err == nil {
		t.Fatal("expected task manager unavailable error")
	}
	execErr, ok := err.(*ToolExecError)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied error code, got %T %#v", err, err)
	}
}

func waitForTaskStatus(t *testing.T, manager runtimepkg.TaskManager, taskID corepkg.TaskID, expected corepkg.TaskStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := manager.Get(context.Background(), taskID)
		if err == nil && task.Status == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := manager.Get(context.Background(), taskID)
	t.Fatalf("expected task %q status %q before timeout; current=%q", taskID, expected, task.Status)
}
