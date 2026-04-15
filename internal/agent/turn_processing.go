package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	"bytemind/internal/tokenusage"
)

type turnProcessParams struct {
	Session          *session.Session
	RunMode          planpkg.AgentMode
	Messages         []llm.Message
	Assets           map[llm.AssetID]llm.ImageAsset
	AllowedToolNames []string
	DeniedToolNames  []string
	AllowedTools     map[string]struct{}
	DeniedTools      map[string]struct{}
	SequenceTracker  *runtimepkg.ToolSequenceTracker
	ExecutedTools    *[]string
	Out              io.Writer
}

func (r *Runner) processTurn(ctx context.Context, p turnProcessParams) (string, bool, error) {
	if r.registry == nil {
		return "", false, fmt.Errorf("tool registry is unavailable")
	}
	filteredTools := r.registry.DefinitionsForModeWithFilters(p.RunMode, p.AllowedToolNames, p.DeniedToolNames)
	request := contextpkg.BuildChatRequest(contextpkg.ChatRequestInput{
		Model:       r.config.Provider.Model,
		Messages:    p.Messages,
		Tools:       filteredTools,
		Assets:      p.Assets,
		Temperature: 0.2,
	})

	streamedText := false
	turnStart := time.Now()
	reply, err := r.completeTurn(ctx, request, p.Out, &streamedText)
	turnLatency := time.Since(turnStart)
	if err != nil {
		estimatedUsage := tokenusage.ResolveTurnUsage(request, nil)
		r.recordTokenUsage(ctx, p.Session, request, estimatedUsage, turnLatency, false)
		return "", false, err
	}
	reply.Normalize()
	turnUsage := tokenusage.ResolveTurnUsage(request, &reply)
	r.recordTokenUsage(ctx, p.Session, request, turnUsage, turnLatency, true)
	r.emitUsageEvent(p.Session, &turnUsage)

	if len(reply.ToolCalls) == 0 {
		answer, finalizeErr := r.finalizeTurnWithoutTools(p.RunMode, p.Session, reply, p.Out, streamedText)
		return answer, true, finalizeErr
	}

	if err := llm.ValidateMessage(reply); err != nil {
		return "", false, err
	}
	sequenceObservation := p.SequenceTracker.Observe(reply.ToolCalls)
	if sequenceObservation.ReachedThreshold {
		summary := runtimepkg.BuildStopSummary(runtimepkg.StopSummaryInput{
			SessionID:     corepkg.SessionID(p.Session.ID),
			Reason:        fmt.Sprintf("I stopped because the assistant repeated the same tool sequence %d times in a row (%s).", sequenceObservation.RepeatCount, strings.Join(sequenceObservation.UniqueToolNames, ", ")),
			ExecutedTools: *p.ExecutedTools,
		})
		answer, summaryErr := r.finishWithSummary(p.Session, summary, p.Out, streamedText)
		return answer, true, summaryErr
	}

	p.Session.Messages = append(p.Session.Messages, reply)
	if r.store != nil {
		if err := r.store.Save(p.Session); err != nil {
			return "", false, err
		}
	}

	if streamedText && p.Out != nil {
		_, _ = io.WriteString(p.Out, "\n")
	}
	for _, call := range reply.ToolCalls {
		*p.ExecutedTools = append(*p.ExecutedTools, call.Function.Name)
		if err := r.executeToolCall(ctx, p.Session, p.RunMode, call, p.Out, p.AllowedTools, p.DeniedTools); err != nil {
			return "", false, err
		}
	}
	return "", false, nil
}
