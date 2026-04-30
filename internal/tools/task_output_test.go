package tools

import (
	"context"
	"encoding/json"
	"testing"

	corepkg "bytemind/internal/core"
	runtimepkg "bytemind/internal/runtime"
)

func TestTaskOutputToolReadsIncrementalOutput(t *testing.T) {
	manager := runtimepkg.NewInMemoryTaskManager(runtimepkg.WithTaskExecutor(func(_ context.Context, _ runtimepkg.Task) ([]byte, error) {
		return []byte("ok"), nil
	}))
	taskID, err := manager.Submit(context.Background(), runtimepkg.TaskSpec{
		SessionID: "sess-1",
		Name:      "subagent",
		Kind:      "subagent",
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if _, err := manager.Wait(context.Background(), taskID); err != nil {
		t.Fatalf("wait task: %v", err)
	}

	tool := TaskOutputTool{}
	out, runErr := tool.Run(context.Background(), []byte(`{"task_id":"`+string(taskID)+`","offset":0,"limit":10}`), &ExecutionContext{
		TaskManager: manager,
	})
	if runErr != nil {
		t.Fatalf("task_output run failed: %v", runErr)
	}

	var result struct {
		OK         bool   `json:"ok"`
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Offset     uint64 `json:"offset"`
		NextOffset uint64 `json:"next_offset"`
		HasMore    bool   `json:"has_more"`
		Items      []struct {
			Offset    uint64 `json:"offset"`
			Payload   string `json:"payload"`
			Timestamp string `json:"timestamp"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal task_output result: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result.TaskID != string(taskID) {
		t.Fatalf("expected task id %q, got %q", taskID, result.TaskID)
	}
	if result.TaskStatus != string(corepkg.TaskCompleted) {
		t.Fatalf("expected completed task status, got %q", result.TaskStatus)
	}
	if result.Offset != 0 {
		t.Fatalf("expected offset 0, got %d", result.Offset)
	}
	if len(result.Items) == 0 {
		t.Fatalf("expected at least one output item, got %#v", result)
	}
	if result.Items[0].Payload == "" {
		t.Fatalf("expected non-empty payload, got %#v", result.Items[0])
	}
}

func TestTaskOutputToolValidatesArgs(t *testing.T) {
	tool := TaskOutputTool{}
	_, err := tool.Run(context.Background(), []byte(`{"task_id":"","offset":-1}`), &ExecutionContext{
		TaskManager: runtimepkg.NewInMemoryTaskManager(),
	})
	if err == nil {
		t.Fatal("expected invalid args error")
	}
	execErr, ok := err.(*ToolExecError)
	if !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("expected invalid args error code, got %T %#v", err, err)
	}
}

func TestTaskOutputToolReturnsInvalidArgsForUnknownTask(t *testing.T) {
	tool := TaskOutputTool{}
	_, err := tool.Run(context.Background(), []byte(`{"task_id":"missing-task"}`), &ExecutionContext{
		TaskManager: runtimepkg.NewInMemoryTaskManager(),
	})
	if err == nil {
		t.Fatal("expected task not found error")
	}
	execErr, ok := err.(*ToolExecError)
	if !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("expected invalid args error code, got %T %#v", err, err)
	}
}

func TestTaskOutputToolRequiresTaskManager(t *testing.T) {
	tool := TaskOutputTool{}
	_, err := tool.Run(context.Background(), []byte(`{"task_id":"task-1"}`), &ExecutionContext{})
	if err == nil {
		t.Fatal("expected task manager unavailable error")
	}
	execErr, ok := err.(*ToolExecError)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied error code, got %T %#v", err, err)
	}
}
