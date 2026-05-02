package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
)

type TaskOutputTool struct{}

type taskOutputItem struct {
	Offset    uint64 `json:"offset"`
	Payload   string `json:"payload"`
	Timestamp string `json:"timestamp,omitempty"`
}

type taskOutputResult struct {
	OK         bool             `json:"ok"`
	TaskID     string           `json:"task_id"`
	TaskStatus string           `json:"task_status,omitempty"`
	ErrorCode  string           `json:"error_code,omitempty"`
	Offset     uint64           `json:"offset"`
	NextOffset uint64           `json:"next_offset"`
	HasMore    bool             `json:"has_more"`
	Items      []taskOutputItem `json:"items"`
}

func (TaskOutputTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "task_output",
			Description: "Read runtime task incremental output by task_id, offset, and limit.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "Runtime task identifier returned by an async tool call.",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Read offset. Defaults to 0.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of log entries to read.",
					},
				},
				"required": []string{"task_id"},
			},
		},
	}
}

func (TaskOutputTool) Spec() ToolSpec {
	return ToolSpec{
		Name:            "task_output",
		ReadOnly:        true,
		ConcurrencySafe: true,
		Destructive:     false,
		SafetyClass:     SafetyClassModerate,
		StrictArgs:      true,
		AllowedModes:    []planpkg.AgentMode{planpkg.ModeBuild, planpkg.ModePlan},
		DefaultTimeoutS: 10,
		MaxTimeoutS:     60,
		MaxResultChars:  128 * 1024,
	}
}

func (TaskOutputTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if execCtx == nil || execCtx.TaskManager == nil {
		return "", NewToolExecError(ToolErrorPermissionDenied, "task manager is unavailable", false, nil)
	}
	logReader, ok := execCtx.TaskManager.(runtimepkg.LogReader)
	if !ok {
		return "", NewToolExecError(ToolErrorPermissionDenied, "task output is unavailable: task manager does not support incremental reads", false, nil)
	}

	var args struct {
		TaskID string `json:"task_id"`
		Offset *int64 `json:"offset"`
		Limit  *int   `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	}

	taskID := corepkg.TaskID(strings.TrimSpace(args.TaskID))
	if taskID == "" {
		return "", NewToolExecError(ToolErrorInvalidArgs, "task_id is required", false, nil)
	}

	offset := uint64(0)
	if args.Offset != nil {
		if *args.Offset < 0 {
			return "", NewToolExecError(ToolErrorInvalidArgs, "offset must be >= 0", false, nil)
		}
		offset = uint64(*args.Offset)
	}

	limit := 0
	if args.Limit != nil {
		if *args.Limit <= 0 {
			return "", NewToolExecError(ToolErrorInvalidArgs, "limit must be > 0", false, nil)
		}
		limit = *args.Limit
	}

	task, getErr := execCtx.TaskManager.Get(ctx, taskID)
	if getErr != nil {
		return "", normalizeTaskManagerError(getErr)
	}
	if accessErr := ensureTaskOutputSessionAccess(task, execCtx); accessErr != nil {
		return "", accessErr
	}

	entries, nextOffset, hasMore, readErr := logReader.ReadIncrement(ctx, taskID, offset, limit)
	if readErr != nil {
		return "", normalizeTaskManagerError(readErr)
	}

	items := make([]taskOutputItem, 0, len(entries))
	for _, entry := range entries {
		item := taskOutputItem{
			Offset:  entry.Offset,
			Payload: string(entry.Payload),
		}
		if !entry.Timestamp.IsZero() {
			item.Timestamp = entry.Timestamp.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}

	result := taskOutputResult{
		OK:         true,
		TaskID:     strings.TrimSpace(string(taskID)),
		TaskStatus: string(task.Status),
		ErrorCode:  strings.TrimSpace(task.ErrorCode),
		Offset:     offset,
		NextOffset: nextOffset,
		HasMore:    hasMore,
		Items:      items,
	}
	if result.Items == nil {
		result.Items = []taskOutputItem{}
	}

	return toJSON(result)
}

func ensureTaskOutputSessionAccess(task runtimepkg.Task, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Session == nil {
		return NewToolExecError(ToolErrorPermissionDenied, "task output is unavailable: session context is missing", false, nil)
	}
	currentSessionID := strings.TrimSpace(execCtx.Session.ID)
	if currentSessionID == "" {
		return NewToolExecError(ToolErrorPermissionDenied, "task output is unavailable: current session id is missing", false, nil)
	}
	taskSessionID := strings.TrimSpace(string(task.Spec.SessionID))
	if taskSessionID == "" {
		return NewToolExecError(ToolErrorPermissionDenied, "task output is unavailable: task session id is missing", false, nil)
	}
	if taskSessionID != currentSessionID {
		return NewToolExecError(ToolErrorPermissionDenied, "task output is unavailable: task belongs to another session", false, nil)
	}
	return nil
}

func normalizeTaskManagerError(err error) error {
	if err == nil {
		return nil
	}
	var semantic interface{ Code() string }
	if errors.As(err, &semantic) {
		switch strings.TrimSpace(semantic.Code()) {
		case runtimepkg.ErrorCodeTaskNotFound:
			return NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
		case runtimepkg.ErrorCodeInvalidTransition:
			return NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
		default:
			return NewToolExecError(ToolErrorToolFailed, err.Error(), true, err)
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewToolExecError(ToolErrorTimeout, fmt.Sprintf("task output timed out: %v", err), true, err)
	}
	return NewToolExecError(ToolErrorToolFailed, err.Error(), true, err)
}
