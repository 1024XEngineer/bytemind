package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
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
	sandboxAudit sandboxAuditContext,
	task runtimepkg.Task,
) {
	if task.ID == "" {
		return
	}
	metadata := map[string]string{
		"tool_name": toolName,
		"status":    string(task.Status),
	}
	appendSandboxAuditContext(metadata, sandboxAudit)
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
