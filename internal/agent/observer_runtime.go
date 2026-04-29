package agent

import (
	"context"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

func (r *Runner) appendAudit(ctx context.Context, event storagepkg.AuditEvent) {
	if r == nil || r.auditStore == nil {
		return
	}
	_ = r.auditStore.Append(ctx, event)
}

func (r *Runner) appendPromptHistory(sessionID corepkg.SessionID, prompt string, at time.Time) {
	if r == nil || r.promptStore == nil {
		return
	}
	_ = r.promptStore.Append(r.workspace, sessionID, prompt, at)
}

func (r *Runner) emit(event Event) {
	if r.observer == nil {
		return
	}
	r.observer.HandleEvent(event)
}
