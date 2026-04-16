package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
)

func (r *Runner) executeToolCall(
	ctx context.Context,
	sess *session.Session,
	runMode planpkg.AgentMode,
	call llm.ToolCall,
	out io.Writer,
	allowedTools map[string]struct{},
	deniedTools map[string]struct{},
) error {
	if r.executor == nil {
		return fmt.Errorf("tool executor is unavailable")
	}
	traceID := buildToolTraceID(call)
	sessionID := corepkg.SessionID(sess.ID)

	r.emit(Event{
		Type:          EventToolCallStarted,
		SessionID:     sessionID,
		ToolName:      call.Function.Name,
		ToolArguments: call.Function.Arguments,
	})
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TraceID:   traceID,
		Actor:     "agent",
		Action:    "tool_execute_start",
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
		},
	})
	if out != nil {
		_, _ = io.WriteString(out, ansiBold+ansiCyan+"tool>"+ansiReset+" "+call.Function.Name+"\n")
	}

	execStartedAt := time.Now()
	execution, runtimeErr := r.runtime.RunSync(ctx, RuntimeTaskRequest{
		SessionID: sessionID,
		TraceID:   traceID,
		Name:      call.Function.Name,
		Kind:      "tool",
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
		},
		Execute: func(execCtx context.Context) ([]byte, error) {
			output, err := r.executor.ExecuteForMode(execCtx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
				Workspace:      r.workspace,
				ApprovalPolicy: r.config.ApprovalPolicy,
				Approval:       r.approval,
				Session:        sess,
				TaskManager:    r.taskManager,
				Extensions:     r.extensions,
				Mode:           runMode,
				Stdin:          r.stdin,
				Stdout:         r.stdout,
				AllowedTools:   allowedTools,
				DeniedTools:    deniedTools,
			})
			return []byte(output), err
		},
		OnTaskStateChanged: func(task runtimepkg.Task) {
			r.appendTaskStateAudit(ctx, sessionID, traceID, call.Function.Name, task)
		},
	})

	result := string(execution.Result.Output)
	execErr := execution.ExecutionError
	if runtimeErr != nil && execution.Result.TaskID == "" {
		execErr = runtimeErr
	}
	if execErr == nil && execution.Result.TaskID != "" && execution.Result.Status != corepkg.TaskCompleted {
		execErr = runtimeTaskResultError{
			status:    execution.Result.Status,
			errorCode: execution.Result.ErrorCode,
		}
	}
	if execErr == nil && runtimeErr != nil {
		execErr = runtimeErr
	}

	if execErr != nil {
		result = marshalToolResult(map[string]any{
			"ok":    false,
			"error": execErr.Error(),
		})
	}
	if out != nil {
		r.renderToolFeedback(out, call.Function.Name, result)
	}

	errText := ""
	if execErr != nil {
		errText = execErr.Error()
	}
	r.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  sessionID,
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errText,
	})

	auditResult := "ok"
	if execErr != nil {
		auditResult = "error"
	}
	metadata := map[string]string{
		"tool_name": call.Function.Name,
		"error":     errText,
	}
	if execution.Result.ErrorCode != "" {
		metadata["error_code"] = execution.Result.ErrorCode
	}
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TaskID:    execution.TaskID,
		TraceID:   traceID,
		Actor:     "agent",
		Action:    "tool_execute_result",
		Result:    auditResult,
		LatencyMS: time.Since(execStartedAt).Milliseconds(),
		Metadata:  metadata,
	})

	toolMessage := llm.NewToolResultMessage(call.ID, result)
	if err := llm.ValidateMessage(toolMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, toolMessage)
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return err
		}
	}
	if call.Function.Name == "update_plan" {
		r.emit(Event{
			Type:      EventPlanUpdated,
			SessionID: sessionID,
			Plan:      planpkg.CloneState(sess.Plan),
		})
	}
	return nil
}

func buildToolTraceID(call llm.ToolCall) corepkg.TraceID {
	if id := strings.TrimSpace(call.ID); id != "" {
		return corepkg.TraceID(id)
	}
	return corepkg.TraceID(fmt.Sprintf("tool-%d", time.Now().UTC().UnixNano()))
}

func (r *Runner) appendTaskStateAudit(
	ctx context.Context,
	sessionID corepkg.SessionID,
	traceID corepkg.TraceID,
	toolName string,
	task runtimepkg.Task,
) {
	if task.ID == "" {
		return
	}
	metadata := map[string]string{
		"tool_name": toolName,
		"status":    string(task.Status),
	}
	if task.ErrorCode != "" {
		metadata["error_code"] = task.ErrorCode
	}
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TaskID:    task.ID,
		TraceID:   traceID,
		Actor:     "runtime",
		Action:    "task_state_changed",
		Result:    string(task.Status),
		Metadata:  metadata,
	})
}
