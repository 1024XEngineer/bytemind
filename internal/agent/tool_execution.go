package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
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
	if r.policyGateway == nil {
		return fmt.Errorf("policy gateway is unavailable")
	}

	decision, err := r.policyGateway.DecideTool(ctx, ToolDecisionInput{
		ToolName:       call.Function.Name,
		AllowedTools:   allowedTools,
		DeniedTools:    deniedTools,
		ApprovalPolicy: r.config.ApprovalPolicy,
		SafetyClass:    r.toolSafetyClass(call.Function.Name),
	})
	if err != nil {
		return err
	}
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID:  corepkg.SessionID(sess.ID),
		Actor:      "agent",
		Action:     "permission_decision",
		Decision:   decision.Decision,
		ReasonCode: decision.ReasonCode,
		RiskLevel:  decision.RiskLevel,
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
			"reason":    decision.Reason,
		},
	})

	if decision.Decision == corepkg.DecisionDeny {
		return r.handleRejectedToolCall(ctx, sess, call, out, decision)
	}

	r.emit(Event{
		Type:          EventToolCallStarted,
		SessionID:     corepkg.SessionID(sess.ID),
		ToolName:      call.Function.Name,
		ToolArguments: call.Function.Arguments,
	})
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
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
	result, execErr := r.executor.ExecuteForMode(ctx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
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
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errText,
	})

	auditResult := "ok"
	if execErr != nil {
		auditResult = "error"
	}
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
		Actor:     "agent",
		Action:    "tool_execute_result",
		Result:    auditResult,
		LatencyMS: time.Since(execStartedAt).Milliseconds(),
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
			"error":     errText,
		},
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
			SessionID: corepkg.SessionID(sess.ID),
			Plan:      planpkg.CloneState(sess.Plan),
		})
	}
	return nil
}

func (r *Runner) toolSafetyClass(name string) tools.SafetyClass {
	type toolSpecLookup interface {
		Spec(name string) (tools.ToolSpec, bool)
	}
	lookup, ok := r.registry.(toolSpecLookup)
	if !ok {
		return tools.SafetyClassModerate
	}
	spec, ok := lookup.Spec(name)
	if !ok || spec.SafetyClass == "" {
		return tools.SafetyClassModerate
	}
	return spec.SafetyClass
}

func (r *Runner) handleRejectedToolCall(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
	decision ToolDecision,
) error {
	errorText := fmt.Sprintf("tool %q blocked by policy (%s): %s", call.Function.Name, decision.ReasonCode, decision.Reason)
	if decision.ReasonCode == policyReasonExplicitDeny {
		errorText = fmt.Sprintf("tool %q is unavailable by active skill policy: %s", call.Function.Name, decision.Reason)
	}
	result := marshalToolResult(map[string]any{
		"ok":          false,
		"error":       errorText,
		"decision":    decision.Decision,
		"reason_code": decision.ReasonCode,
	})

	if out != nil {
		r.renderToolFeedback(out, call.Function.Name, result)
	}

	r.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errorText,
	})

	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
		Actor:     "agent",
		Action:    "tool_execute_result",
		Result:    "denied",
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
			"error":     errorText,
			"decision":  string(decision.Decision),
		},
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
	return nil
}
