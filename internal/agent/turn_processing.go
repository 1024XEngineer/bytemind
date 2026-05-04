package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	contextpkg "github.com/1024XEngineer/bytemind/internal/context"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tokenusage"
	"github.com/1024XEngineer/bytemind/internal/tools"
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
	SequenceTracker  *ToolSequenceTracker
	AdaptiveState    *adaptiveTurnState
	ExecutedTools    *[]string
	Approval         tools.ApprovalHandler
	SandboxAudit     sandboxAuditContext
	TaskReport       *TaskReport
	Out              io.Writer
}

func (e *defaultEngine) processTurn(ctx context.Context, p turnProcessParams) (string, bool, error) {
	if e == nil || e.runner == nil {
		return "", false, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	if runner.registry == nil {
		return "", false, fmt.Errorf("tool registry is unavailable")
	}
	filteredTools := runner.registry.DefinitionsForModeWithFilters(p.RunMode, p.AllowedToolNames, p.DeniedToolNames)
	availableToolNames := toolNames(filteredTools)
	request := contextpkg.BuildChatRequest(contextpkg.ChatRequestInput{
		Model:       runner.config.Provider.Model,
		Messages:    p.Messages,
		Tools:       filteredTools,
		Assets:      p.Assets,
		Temperature: 0.2,
	})
	request.Model = runner.modelID()

	streamedText := false
	turnStart := time.Now()
	reply, err := e.completeTurn(ctx, request, p.Out, &streamedText)
	turnLatency := time.Since(turnStart)
	if err != nil {
		estimatedUsage := tokenusage.ResolveTurnUsage(request, nil)
		runner.recordTokenUsage(ctx, p.Session, request, estimatedUsage, turnLatency, false)
		return "", false, err
	}
	reply.Normalize()
	_, cleanedReply, _ := parseAssistantTurnIntent(reply)
	reply = cleanedReply
	turnUsage := tokenusage.ResolveTurnUsage(request, &reply)
	runner.recordTokenUsage(ctx, p.Session, request, turnUsage, turnLatency, true)
	runner.emitUsageEvent(p.Session, &turnUsage)

	if len(reply.ToolCalls) == 0 {
		// No tool calls — finalize the turn. Safety nets only for specific claim patterns.
		latestUser := latestHumanUserMessageText(p.Session.Messages)
		if shouldRepairUnexecutedToolClaimTurn(p.RunMode, reply, p.Session.Messages, availableToolNames) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("unexecuted_tool_claim")
				p.TaskReport.RecordStrategyAdjustment("assistant claimed run_shell was unavailable or timed out without a structured tool call; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("tool-claim repair retries exceeded while waiting for a real structured tool call")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept claiming run_shell was unavailable or timed out without issuing a structured tool call (attempts=%d).", attempt),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildUnexecutedToolClaimRepairInstruction(reply, latestUser, attempt, maxAttempts, availableToolNames))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant claimed shell unavailability/timeout without a structured tool call; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
		localRepoRepairKind, _ := evaluateLocalRepoClaimRepairTurn(p.RunMode, latestUser, reply, p.Session.Messages)
		if localRepoRepairKind != localRepoClaimRepairNone {
			if p.TaskReport != nil {
				switch localRepoRepairKind {
				case localRepoClaimRepairPathUnverified:
					p.TaskReport.RecordStrategyAdjustment("assistant made a concrete local repo claim without directly confirming the referenced path; finalized as-is")
				case localRepoClaimRepairImplementationUnverified:
					p.TaskReport.RecordStrategyAdjustment("assistant concluded the repo already had a runnable implementation based only on weak signals; finalized as-is")
				}
			}
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordProgress()
			}
		}
		if p.AdaptiveState != nil {
			p.AdaptiveState.recordProgress()
		}
		answer, finalizeErr := e.finalizeTurnWithoutTools(p.RunMode, p.Session, reply, p.Out, streamedText)
		return answer, true, finalizeErr
	}
	if p.AdaptiveState != nil {
		p.AdaptiveState.recordProgress()
	}

	if err := llm.ValidateMessage(reply); err != nil {
		return "", false, err
	}
	sequenceObservation := p.SequenceTracker.Observe(reply.ToolCalls)
	if sequenceObservation.ReachedThreshold {
		repeatKind := "exact tool+argument sequence"
		if sequenceObservation.MatchMode == "name_only" {
			repeatKind = "same tool-name sequence (arguments varied)"
		}
		summary := BuildStopSummary(StopSummaryInput{
			SessionID:     corepkg.SessionID(p.Session.ID),
			Reason:        fmt.Sprintf("I stopped because the assistant repeated the %s %d times in a row (%s).", repeatKind, sequenceObservation.RepeatCount, strings.Join(sequenceObservation.UniqueToolNames, ", ")),
			ExecutedTools: *p.ExecutedTools,
			TaskReport:    p.TaskReport,
		})
		answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
		return answer, true, summaryErr
	}

	p.Session.Messages = append(p.Session.Messages, reply)
	if runner.store != nil {
		if err := runner.store.Save(p.Session); err != nil {
			return "", false, err
		}
	}

	if streamedText && p.Out != nil {
		_, _ = io.WriteString(p.Out, "\n")
	}
	for _, call := range reply.ToolCalls {
		*p.ExecutedTools = append(*p.ExecutedTools, call.Function.Name)
		if p.TaskReport != nil {
			p.TaskReport.RecordExecuted(call.Function.Name)
		}
		emitTurnEvent(ctx, TurnEvent{
			Type: TurnEventToolUse,
			Payload: map[string]any{
				"tool_name":      call.Function.Name,
				"tool_arguments": call.Function.Arguments,
				"tool_call_id":   call.ID,
			},
		})
		if err := e.executeToolCall(ctx, p.Session, p.RunMode, call, p.Out, p.AllowedTools, p.DeniedTools, p.Approval, p.SandboxAudit); err != nil {
			return "", false, err
		}
		envelope, ok := latestToolResultEnvelope(p.Session)
		if ok && p.TaskReport != nil {
			if note := systemSandboxFallbackReportEntry(call.Function.Name, envelope); note != "" {
				p.TaskReport.RecordSystemSandboxFallback(note)
			}
		}
		if ok && p.TaskReport != nil && envelope.Status == statusDenied {
			p.TaskReport.RecordDenied(call.Function.Name)
		}
	}
	return "", false, nil
}

func (r *Runner) processTurn(ctx context.Context, p turnProcessParams) (string, bool, error) {
	engine := &defaultEngine{runner: r}
	return engine.processTurn(ctx, p)
}

const (
	statusError                = "error"
	statusDenied               = "denied"
	reasonCodePermissionDenied = "permission_denied"
)

type toolResultEnvelope struct {
	OK            *bool  `json:"ok"`
	Error         string `json:"error"`
	Status        string `json:"status"`
	ReasonCode    string `json:"reason_code"`
	SystemSandbox struct {
		Mode            string `json:"mode"`
		Backend         string `json:"backend"`
		RequiredCapable bool   `json:"required_capable"`
		CapabilityLevel string `json:"capability_level"`
		ShellNetwork    bool   `json:"shell_network_isolation"`
		WorkerNetwork   bool   `json:"worker_network_isolation"`
		Fallback        bool   `json:"fallback"`
		FallbackReason  string `json:"fallback_reason"`
	} `json:"system_sandbox"`
}

func latestToolResultEnvelope(sess *session.Session) (toolResultEnvelope, bool) {
	if sess == nil || len(sess.Messages) == 0 {
		return toolResultEnvelope{}, false
	}
	last := sess.Messages[len(sess.Messages)-1]
	content := strings.TrimSpace(last.Content)
	if content == "" {
		return toolResultEnvelope{}, false
	}
	var envelope toolResultEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return toolResultEnvelope{}, false
	}
	envelope.Status = strings.ToLower(strings.TrimSpace(envelope.Status))
	envelope.ReasonCode = strings.ToLower(strings.TrimSpace(envelope.ReasonCode))
	return envelope, true
}

func systemSandboxFallbackReportEntry(toolName string, envelope toolResultEnvelope) string {
	if !envelope.SystemSandbox.Fallback {
		return ""
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "unknown_tool"
	}
	mode := strings.TrimSpace(envelope.SystemSandbox.Mode)
	backend := strings.TrimSpace(envelope.SystemSandbox.Backend)
	reason := strings.TrimSpace(envelope.SystemSandbox.FallbackReason)
	parts := make([]string, 0, 3)
	if mode != "" {
		parts = append(parts, "mode="+mode)
	}
	if backend != "" {
		parts = append(parts, "backend="+backend)
	}
	parts = append(parts, fmt.Sprintf("required_capable=%t", envelope.SystemSandbox.RequiredCapable))
	if capability := strings.TrimSpace(envelope.SystemSandbox.CapabilityLevel); capability != "" {
		parts = append(parts, "capability_level="+capability)
	}
	parts = append(parts, fmt.Sprintf("shell_network_isolation=%t", envelope.SystemSandbox.ShellNetwork))
	parts = append(parts, fmt.Sprintf("worker_network_isolation=%t", envelope.SystemSandbox.WorkerNetwork))
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}
	if len(parts) == 0 {
		return toolName
	}
	return toolName + " (" + strings.Join(parts, ", ") + ")"
}
