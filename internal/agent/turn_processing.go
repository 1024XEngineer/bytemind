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
	intent, cleanedReply, explicitIntent := parseAssistantTurnIntent(reply)
	reply = cleanedReply
	turnUsage := tokenusage.ResolveTurnUsage(request, &reply)
	runner.recordTokenUsage(ctx, p.Session, request, turnUsage, turnLatency, true)
	runner.emitUsageEvent(p.Session, &turnUsage)

	if len(reply.ToolCalls) == 0 {
		if intent == turnIntentUnknown {
			intent = inferAssistantTurnIntent(reply.Content)
		}
		latestUser := latestHumanUserMessageText(p.Session.Messages)
		if shouldRepairPlanBootstrapTurn(p.RunMode, p.Session.Plan, reply) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("plan_bootstrap_missing_update")
				p.TaskReport.RecordStrategyAdjustment("assistant replied in plan mode before creating any structured plan state; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("plan bootstrap repair retries exceeded while waiting for initial investigation or update_plan")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept replying in plan mode without creating the initial structured plan state first (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildPlanBootstrapRepairInstruction(reply, latestUser, attempt, maxAttempts, p.Session.Messages, availableToolNames))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant replied in plan mode before creating structured plan state; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
		if !hasToolActivitySinceLatestHumanUser(p.Session.Messages) && shouldRepairPlanRevisionTurn(p.RunMode, p.Session.Plan, latestUser, reply) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("plan_revision_missing_update")
				p.TaskReport.RecordStrategyAdjustment("assistant responded to converged-plan refinement feedback without update_plan; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("plan revision repair retries exceeded while waiting for update_plan")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept responding to plan-refinement feedback without updating the structured plan first (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildPlanRevisionRepairInstruction(p.Session.Plan, latestUser, reply, attempt, maxAttempts))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant replied to plan refinement feedback without update_plan; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
		if shouldRepairPlanClarifyTurn(p.RunMode, p.Session.Plan, intent, reply) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("plan_clarify_missing_active_choice")
				p.TaskReport.RecordStrategyAdjustment("assistant asked a plan clarification question without update_plan.active_choice; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("plan clarify repair retries exceeded while waiting for active_choice")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept asking plan clarification questions without storing active_choice first (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildPlanClarifyRepairInstruction(p.Session.Plan, reply, attempt, maxAttempts))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant asked a plan clarification question without active_choice; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
		if shouldRepairPlanDecisionTurn(p.RunMode, p.Session.Plan, intent, reply) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("plan_state_not_updated_after_user_decision")
				p.TaskReport.RecordStrategyAdjustment("assistant acknowledged a plan decision without update_plan; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("plan-state repair retries exceeded while waiting for update_plan")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept acknowledging plan decisions without updating the structured plan state first (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildPlanDecisionRepairInstruction(p.Session.Plan, reply, attempt, maxAttempts))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant acknowledged a plan decision without update_plan; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
		if shouldRepairBuildHandoffTurn(p.RunMode, p.Session.Plan, intent, reply, p.Session.Messages) {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				p.TaskReport.RecordRetry("build_handoff_not_started")
				p.TaskReport.RecordStrategyAdjustment("assistant treated an already-switched build handoff as if plan confirmation were still pending; injected correction prompt")
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("build-handoff repair retries exceeded while waiting for execution to start")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept treating an already-approved build handoff as if plan confirmation were still pending (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildBuildHandoffRepairInstruction(p.Session.Plan, reply, latestUser, attempt, maxAttempts))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant treated an already-switched build handoff as pending plan confirmation; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
		}
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
						Reason:        fmt.Sprintf("I paused because the assistant kept claiming run_shell was unavailable or timed out without issuing a structured tool call (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
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
		localRepoRepairKind, localRepoEvidence := evaluateLocalRepoClaimRepairTurn(p.RunMode, latestUser, reply, p.Session.Messages)
		if localRepoRepairKind != localRepoClaimRepairNone {
			attempt := 0
			maxAttempts := 0
			if p.AdaptiveState != nil {
				p.AdaptiveState.recordNoProgressTurn()
				attempt = p.AdaptiveState.recordSemanticRepairAttempt()
				maxAttempts = p.AdaptiveState.maxSemanticRepairs
			}
			if p.TaskReport != nil {
				p.TaskReport.RecordNoProgressTurn()
				switch localRepoRepairKind {
				case localRepoClaimRepairPathUnverified:
					p.TaskReport.RecordRetry("local_repo_claim_without_direct_path_confirmation")
					p.TaskReport.RecordStrategyAdjustment("assistant made a concrete local repo claim without directly confirming the referenced path; injected correction prompt")
				case localRepoClaimRepairImplementationUnverified:
					p.TaskReport.RecordRetry("local_repo_claim_without_implementation_evidence")
					p.TaskReport.RecordStrategyAdjustment("assistant concluded the repo already had a runnable implementation based only on weak signals; injected correction prompt")
				}
			}
			if p.AdaptiveState != nil {
				if p.AdaptiveState.exceededSemanticRepairLimit() || p.AdaptiveState.exceededNoProgressLimit() {
					if p.TaskReport != nil {
						p.TaskReport.RecordEscalation("local repo claim repair retries exceeded while waiting for direct path confirmation or implementation evidence")
					}
					summary := BuildStopSummary(StopSummaryInput{
						SessionID:     corepkg.SessionID(p.Session.ID),
						Reason:        fmt.Sprintf("I paused because the assistant kept making local repository claims without enough direct evidence (attempts=%d, explicit_intent=%t).", attempt, explicitIntent),
						ExecutedTools: *p.ExecutedTools,
						TaskReport:    p.TaskReport,
					})
					answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
					return answer, true, summaryErr
				}
				p.AdaptiveState.schedulePendingControlNote(buildLocalRepoClaimRepairInstruction(localRepoRepairKind, reply, latestUser, attempt, maxAttempts, localRepoEvidence))
			}
			if p.Out != nil {
				fmt.Fprintf(p.Out, "%sassistant made a concrete local repo claim without enough direct evidence; retrying with a correction prompt%s\n", ansiDim, ansiReset)
			}
			return "", false, nil
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
			if err := e.appendSkippedDependencyResult(ctx, p.Session, skippedCall, p.Out, p.SandboxAudit); err != nil {
				return "", false, err
			}
			if p.TaskReport == nil {
				continue
			}
			if skippedEnvelope, ok := latestToolResultEnvelope(p.Session); ok {
				if note := systemSandboxFallbackReportEntry(skippedCall.Function.Name, skippedEnvelope); note != "" {
					p.TaskReport.RecordSystemSandboxFallback(note)
				}
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

func (e *defaultEngine) appendSkippedDependencyResult(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
	sandboxAudit sandboxAuditContext,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	errText := fmt.Sprintf("%s: tool %q was skipped because a prior approval-required action was denied in away mode", reasonCodeDeniedDependency, call.Function.Name)
	payload := map[string]any{
		"ok":          false,
		"error":       errText,
		"status":      statusSkipped,
		"reason_code": reasonCodeDeniedDependency,
	}
	if systemSandbox := systemSandboxResultPayload(sandboxAudit); len(systemSandbox) != 0 {
		payload["system_sandbox"] = systemSandbox
	}
	result := marshalToolResult(payload)
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
