package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tokenusage"
)

type TokenRealtimeSnapshot struct {
	SessionID            corepkg.SessionID
	SessionInputTokens   int64
	SessionOutputTokens  int64
	SessionContextTokens int64
	SessionTotalTokens   int64
	GlobalTotalTokens    int64
	CurrentTPS           float64
	PeakTPS              float64
	ActiveSessions       int
	ErrorRate            float64
	AvgLatency           time.Duration
	GeneratedAt          time.Time
}

func (r *Runner) HasTokenManager() bool {
	return r != nil && r.tokenManager != nil
}

func (r *Runner) TokenRealtimeEnabled() bool {
	return r != nil && r.tokenManager != nil && r.config.TokenUsage.EnableRealtime
}

func (r *Runner) GetTokenRealtimeSnapshot(sessionID corepkg.SessionID) (TokenRealtimeSnapshot, error) {
	var snapshot TokenRealtimeSnapshot
	if r == nil || r.tokenManager == nil {
		return snapshot, fmt.Errorf("token manager unavailable")
	}
	realtime, err := r.tokenManager.GetRealtimeStats()
	if err != nil {
		return snapshot, err
	}
	snapshot.GlobalTotalTokens = realtime.TotalTokens
	snapshot.CurrentTPS = realtime.Metrics.CurrentTPS
	snapshot.PeakTPS = realtime.Metrics.PeakTPS
	snapshot.ActiveSessions = realtime.Metrics.ActiveSessions
	snapshot.ErrorRate = realtime.Metrics.ErrorRate
	snapshot.AvgLatency = realtime.Metrics.Latency
	snapshot.GeneratedAt = realtime.GeneratedAt
	snapshot.SessionID = corepkg.SessionID(strings.TrimSpace(string(sessionID)))

	if snapshot.SessionID != "" {
		for _, stats := range realtime.Sessions {
			if stats == nil || stats.SessionID != string(snapshot.SessionID) {
				continue
			}
			snapshot.SessionInputTokens = stats.InputTokens
			snapshot.SessionOutputTokens = stats.OutputTokens
			snapshot.SessionTotalTokens = stats.TotalTokens
			break
		}
	}
	return snapshot, nil
}

func (r *Runner) emitUsageEvent(sess *session.Session, usage *llm.Usage) {
	if sess == nil || usage == nil {
		return
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ContextTokens == 0 && usage.TotalTokens == 0 {
		return
	}
	r.emit(Event{
		Type:      EventUsageUpdated,
		SessionID: corepkg.SessionID(sess.ID),
		Usage:     *usage,
	})
}

func (r *Runner) recordTokenUsage(ctx context.Context, sess *session.Session, request llm.ChatRequest, usage llm.Usage, latency time.Duration, success bool) {
	if r.tokenManager == nil || sess == nil {
		return
	}

	req := &tokenusage.TokenRecordRequest{
		SessionID:    sess.ID,
		ModelName:    request.Model,
		InputTokens:  int64(max(0, usage.InputTokens+usage.ContextTokens)),
		OutputTokens: int64(max(0, usage.OutputTokens)),
		RequestID:    time.Now().UTC().Format("20060102150405.000000000"),
		Latency:      latency,
		Success:      success,
		Metadata: map[string]string{
			"workspace": sess.Workspace,
		},
	}
	if err := r.tokenManager.RecordTokenUsage(ctx, req); err != nil && r.stdout != nil {
		fmt.Fprintf(r.stdout, "%swarning%s token usage record failed: %v\n", ansiDim, ansiReset, err)
	}
}

func (r *Runner) Close() error {
	if r == nil || r.tokenManager == nil {
		return nil
	}
	return r.tokenManager.Close()
}
