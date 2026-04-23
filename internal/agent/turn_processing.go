package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
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
	intent, cleanedReply, explicitIntent := parseAssistantTurnIntent(reply)
	reply = cleanedReply
	turnUsage := tokenusage.ResolveTurnUsage(request, &reply)
	runner.recordTokenUsage(ctx, p.Session, request, turnUsage, turnLatency, true)
	runner.emitUsageEvent(p.Session, &turnUsage)

	if len(reply.ToolCalls) == 0 {
		if intent == turnIntentUnknown {
			intent = inferAssistantTurnIntent(reply.Content)
		}
		switch intent {
		case turnIntentContinueWork:
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("missing_structured_tool_call")
				p.TaskReport.RecordStrategyAdjustment("assistant declared continue_work without structured tool calls; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("semantic repair retries exceeded while waiting for structured tool calls")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept signaling ongoing work without structured tool calls (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildSemanticRepairInstruction(reply, attempt, maxAttempts))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant indicated ongoing work but emitted no structured tool calls; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		case turnIntentAskUser, turnIntentFinalize:
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordProgress()
			}
		default:
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordProgress()
			}
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
	for index, call := range reply.ToolCalls {
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
		if !isAwayPermissionDenied(envelope, runner.config.ApprovalMode) {
			continue
		}

		if p.TaskReport != nil {
			p.TaskReport.RecordDenied(call.Function.Name)
		}

		remaining := reply.ToolCalls[index+1:]
		failFast := normalizeAwayPolicy(runner.config.AwayPolicy) == awayPolicyFailFast
		for _, skippedCall := range remaining {
			if p.TaskReport != nil {
				p.TaskReport.RecordSkippedDueToDeniedDependency(skippedCall.Function.Name)
			}
			if failFast {
				continue
			}
			if err := e.appendSkippedDependencyResult(ctx, p.Session, skippedCall, p.Out); err != nil {
				return "", false, err
			}
		}
		if failFast {
			return "", false, fmt.Errorf("away_policy=fail_fast stopped run after permission denial")
		}
		break
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
	statusSkipped              = "skipped"
	reasonCodePermissionDenied = "permission_denied"
	reasonCodeDeniedDependency = "denied_dependency"
	approvalModeInteractive    = "interactive"
	approvalModeAway           = "away"
	awayPolicyAutoDenyContinue = "auto_deny_continue"
	awayPolicyFailFast         = "fail_fast"
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

func isAwayPermissionDenied(envelope toolResultEnvelope, approvalMode string) bool {
	if normalizeApprovalMode(approvalMode) != approvalModeAway {
		return false
	}
	return envelope.Status == statusDenied && envelope.ReasonCode == reasonCodePermissionDenied
}

func normalizeApprovalMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return approvalModeInteractive
	}
	return mode
}

func normalizeAwayPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return awayPolicyAutoDenyContinue
	}
	return policy
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
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}
	if len(parts) == 0 {
		return toolName
	}
	return toolName + " (" + strings.Join(parts, ", ") + ")"
}

func (e *defaultEngine) appendSkippedDependencyResult(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	errText := fmt.Sprintf("%s: tool %q was skipped because a prior approval-required action was denied in away mode", reasonCodeDeniedDependency, call.Function.Name)
	result := marshalToolResult(map[string]any{
		"ok":          false,
		"error":       errText,
		"status":      statusSkipped,
		"reason_code": reasonCodeDeniedDependency,
	})
	if out != nil {
		runner.renderToolFeedback(out, call.Function.Name, result)
	}

	runner.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errText,
	})
	emitTurnEvent(ctx, TurnEvent{
		Type: TurnEventToolResult,
		Payload: map[string]any{
			"tool_name":    call.Function.Name,
			"tool_call_id": call.ID,
			"tool_result":  result,
			"error":        errText,
		},
	})

	toolMessage := llm.NewToolResultMessage(call.ID, result)
	if err := llm.ValidateMessage(toolMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, toolMessage)
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	return nil
}
