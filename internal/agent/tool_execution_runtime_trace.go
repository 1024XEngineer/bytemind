package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	runtimepkg "bytemind/internal/runtime"
	storagepkg "bytemind/internal/storage"
)

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
