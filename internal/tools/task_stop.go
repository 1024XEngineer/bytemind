package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
)

type TaskStopTool struct{}

type taskStopResult struct {
	OK         bool   `json:"ok"`
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	TaskStatus string `json:"task_status,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
}

func (TaskStopTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "task_stop",
			Description: "Stop a runtime task by task_id.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "Runtime task identifier returned by an async tool call.",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Optional stop reason for audit.",
					},
				},
				"required": []string{"task_id"},
			},
		},
	}
}

func (TaskStopTool) Spec() ToolSpec {
	return ToolSpec{
		Name:            "task_stop",
		ReadOnly:        false,
		ConcurrencySafe: true,
		Destructive:     false,
		SafetyClass:     SafetyClassSensitive,
		StrictArgs:      true,
		AllowedModes:    []planpkg.AgentMode{planpkg.ModeBuild, planpkg.ModePlan},
		DefaultTimeoutS: 10,
		MaxTimeoutS:     60,
		MaxResultChars:  32 * 1024,
	}
}

func (TaskStopTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if execCtx == nil || execCtx.TaskManager == nil {
		return "", NewToolExecError(ToolErrorPermissionDenied, "task manager is unavailable", false, nil)
	}

	var args struct {
		TaskID string `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	}

	taskID := corepkg.TaskID(strings.TrimSpace(args.TaskID))
	if taskID == "" {
		return "", NewToolExecError(ToolErrorInvalidArgs, "task_id is required", false, nil)
	}
	reason := strings.TrimSpace(args.Reason)
	if reason == "" {
		reason = "task_stop_requested"
	}

	if cancelErr := execCtx.TaskManager.Cancel(ctx, taskID, reason); cancelErr != nil {
		var semantic interface{ Code() string }
		if errors.As(cancelErr, &semantic) && strings.TrimSpace(semantic.Code()) == runtimepkg.ErrorCodeInvalidTransition {
			if task, getErr := execCtx.TaskManager.Get(ctx, taskID); getErr == nil && runtimepkg.IsTerminalTaskStatus(task.Status) {
				result := taskStopResult{
					OK:         true,
					TaskID:     strings.TrimSpace(string(taskID)),
					Status:     "already_terminal",
					TaskStatus: string(task.Status),
					ErrorCode:  strings.TrimSpace(task.ErrorCode),
				}
				return toJSON(result)
			}
		}
		return "", normalizeTaskManagerError(cancelErr)
	}

	result := taskStopResult{
		OK:     true,
		TaskID: strings.TrimSpace(string(taskID)),
		Status: "stop_requested",
	}
	if task, getErr := execCtx.TaskManager.Get(ctx, taskID); getErr == nil {
		result.TaskStatus = string(task.Status)
		result.ErrorCode = strings.TrimSpace(task.ErrorCode)
	}

	return toJSON(result)
}
