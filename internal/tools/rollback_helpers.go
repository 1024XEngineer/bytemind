package tools

import (
	"context"
	"strings"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
)

type rollbackTracker struct {
	store *rollbackpkg.Store
	op    *rollbackpkg.Operation
	roots []string
}

func beginRollbackOperation(ctx context.Context, execCtx *ExecutionContext, toolName string, targets []rollbackpkg.FileTarget) (*rollbackTracker, error) {
	if !rollbackEnabled(execCtx) {
		return nil, nil
	}
	if len(targets) == 0 {
		return nil, nil
	}
	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		return nil, err
	}
	op, err := store.Begin(ctx, rollbackpkg.BeginOptions{
		Workspace: strings.TrimSpace(execCtx.Workspace),
		SessionID: rollbackSessionID(execCtx),
		TraceID:   strings.TrimSpace(execCtx.RunID),
		ToolName:  toolName,
		Actor:     "agent",
	}, targets)
	if err != nil {
		return nil, err
	}
	return &rollbackTracker{
		store: store,
		op:    op,
		roots: writableRootsFromExecContext(execCtx),
	}, nil
}

func rollbackEnabled(execCtx *ExecutionContext) bool {
	if execCtx == nil {
		return false
	}
	return strings.TrimSpace(execCtx.RunID) != ""
}

func rollbackSessionID(execCtx *ExecutionContext) string {
	if execCtx == nil || execCtx.Session == nil {
		return ""
	}
	return strings.TrimSpace(execCtx.Session.ID)
}

func (t *rollbackTracker) commit(ctx context.Context) (string, error) {
	if t == nil || t.store == nil || t.op == nil {
		return "", nil
	}
	if err := t.store.Commit(ctx, t.op); err != nil {
		restoreErr := t.store.AbortAndRestore(ctx, t.op, err.Error(), t.roots...)
		if restoreErr != nil {
			return "", restoreErr
		}
		return "", err
	}
	return t.op.OperationID, nil
}

func (t *rollbackTracker) abort(ctx context.Context, reason string) {
	if t == nil || t.store == nil || t.op == nil {
		return
	}
	_ = t.store.AbortAndRestore(ctx, t.op, reason, t.roots...)
}
