package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	configpkg "bytemind/internal/config"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	sandboxpkg "bytemind/internal/sandbox"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
)

func (e *defaultEngine) executeToolCall(
	ctx context.Context,
	sess *session.Session,
	runMode planpkg.AgentMode,
	call llm.ToolCall,
	out io.Writer,
	allowedTools map[string]struct{},
	deniedTools map[string]struct{},
	approval tools.ApprovalHandler,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	if runner.executor == nil {
		return fmt.Errorf("tool executor is unavailable")
	}
	if runner.policyGateway == nil {
		return fmt.Errorf("policy gateway is unavailable")
	}
	if runner.runtime == nil {
		return fmt.Errorf("runtime gateway is unavailable")
	}

	traceID := buildToolTraceID(call)
	sessionID := corepkg.SessionID(sess.ID)

	decision, err := runner.policyGateway.DecideTool(ctx, ToolDecisionInput{
		ToolName:       call.Function.Name,
		AllowedTools:   allowedTools,
		DeniedTools:    deniedTools,
		ApprovalPolicy: runner.config.ApprovalPolicy,
		SafetyClass:    e.toolSafetyClass(call.Function.Name),
	})
	if err != nil {
		return err
	}
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID:  sessionID,
		TraceID:    traceID,
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
		return e.handleRejectedToolCall(ctx, sess, call, out, decision)
	}

	runner.emit(Event{
		Type:          EventToolCallStarted,
		SessionID:     sessionID,
		ToolName:      call.Function.Name,
		ToolArguments: call.Function.Arguments,
	})
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TraceID:   traceID,
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
	execution, runtimeErr := runner.runtime.RunSync(ctx, RuntimeTaskRequest{
		SessionID: sessionID,
		TraceID:   traceID,
		Name:      call.Function.Name,
		Kind:      "tool",
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
		},
		Execute: func(execCtx context.Context) ([]byte, error) {
			sandboxRoots := buildSandboxRoots(runner.workspace, runner.config.WritableRoots)
			output, err := runner.executor.ExecuteForMode(execCtx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
				Workspace:        runner.workspace,
				WritableRoots:    runner.config.WritableRoots,
				ApprovalPolicy:   runner.config.ApprovalPolicy,
				ApprovalMode:     runner.config.ApprovalMode,
				AwayPolicy:       runner.config.AwayPolicy,
				SandboxEnabled:   runner.config.SandboxEnabled,
				LeaseID:          fmt.Sprintf("session-%s", sess.ID),
				RunID:            fmt.Sprintf("trace-%s", traceID),
				FSRead:           append([]string(nil), sandboxRoots...),
				FSWrite:          append([]string(nil), sandboxRoots...),
				ExecAllowlist:    toSandboxExecRules(runner.config.ExecAllowlist),
				NetworkAllowlist: toSandboxNetworkRules(runner.config.NetworkAllowlist),
				Approval:         approval,
				Session:          sess,
				TaskManager:      runner.taskManager,
				Extensions:       runner.extensions,
				Mode:             runMode,
				Stdin:            runner.stdin,
				Stdout:           runner.stdout,
				AllowedTools:     allowedTools,
				DeniedTools:      deniedTools,
			})
			return []byte(output), err
		},
		OnTaskStateChanged: func(task runtimepkg.Task) {
			runner.appendTaskStateAudit(ctx, sessionID, traceID, call.Function.Name, task)
		},
	})

	result := string(execution.Result.Output)
	execErr := execution.ExecutionError
	if runtimeErr != nil && execution.Result.TaskID == "" {
		execErr = runtimeErr
	}
	if execErr == nil && execution.Result.TaskID != "" && execution.Result.Status != corepkg.TaskCompleted {
		execErr = runtimeTaskResultError{
			status:    execution.Result.Status,
			errorCode: execution.Result.ErrorCode,
		}
	}
	if execErr == nil && runtimeErr != nil {
		execErr = runtimeErr
	}

	if execErr != nil {
		status, reasonCode := classifyToolExecutionError(execErr)
		result = marshalToolResult(map[string]any{
			"ok":          false,
			"error":       execErr.Error(),
			"status":      status,
			"reason_code": reasonCode,
		})
	}
	if out != nil {
		runner.renderToolFeedback(out, call.Function.Name, result)
	}

	errText := ""
	if execErr != nil {
		errText = execErr.Error()
	}
	runner.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  sessionID,
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

	auditResult := "ok"
	if execErr != nil {
		auditResult = "error"
	}
	metadata := map[string]string{
		"tool_name": call.Function.Name,
		"error":     errText,
	}
	if execution.Result.ErrorCode != "" {
		metadata["error_code"] = execution.Result.ErrorCode
	}
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TaskID:    execution.TaskID,
		TraceID:   traceID,
		Actor:     "agent",
		Action:    "tool_execute_result",
		Result:    auditResult,
		LatencyMS: time.Since(execStartedAt).Milliseconds(),
		Metadata:  metadata,
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
	if call.Function.Name == "update_plan" {
		runner.emit(Event{
			Type:      EventPlanUpdated,
			SessionID: sessionID,
			Plan:      planpkg.CloneState(sess.Plan),
		})
	}
	return nil
}

func (e *defaultEngine) toolSafetyClass(name string) tools.SafetyClass {
	if e == nil || e.runner == nil {
		return tools.SafetyClassModerate
	}
	runner := e.runner

	type toolSpecLookup interface {
		Spec(name string) (tools.ToolSpec, bool)
	}
	lookup, ok := runner.registry.(toolSpecLookup)
	if !ok {
		return tools.SafetyClassModerate
	}
	spec, ok := lookup.Spec(name)
	if !ok || spec.SafetyClass == "" {
		return tools.SafetyClassModerate
	}
	return spec.SafetyClass
}

func (e *defaultEngine) handleRejectedToolCall(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
	decision ToolDecision,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	errorText := fmt.Sprintf("tool %q blocked by policy (%s): %s", call.Function.Name, decision.ReasonCode, decision.Reason)
	if decision.ReasonCode == policyReasonExplicitDeny {
		errorText = fmt.Sprintf("tool %q is unavailable by active skill policy: %s", call.Function.Name, decision.Reason)
	}
	result := marshalToolResult(map[string]any{
		"ok":          false,
		"error":       errorText,
		"status":      statusDenied,
		"decision":    decision.Decision,
		"reason_code": decision.ReasonCode,
	})

	if out != nil {
		runner.renderToolFeedback(out, call.Function.Name, result)
	}

	runner.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errorText,
	})
	emitTurnEvent(ctx, TurnEvent{
		Type: TurnEventToolResult,
		Payload: map[string]any{
			"tool_name":    call.Function.Name,
			"tool_call_id": call.ID,
			"tool_result":  result,
			"error":        errorText,
		},
	})

	runner.appendAudit(ctx, storagepkg.AuditEvent{
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
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) executeToolCall(
	ctx context.Context,
	sess *session.Session,
	runMode planpkg.AgentMode,
	call llm.ToolCall,
	out io.Writer,
	allowedTools map[string]struct{},
	deniedTools map[string]struct{},
	approval tools.ApprovalHandler,
) error {
	engine := &defaultEngine{runner: r}
	return engine.executeToolCall(ctx, sess, runMode, call, out, allowedTools, deniedTools, approval)
}

func (r *Runner) toolSafetyClass(name string) tools.SafetyClass {
	engine := &defaultEngine{runner: r}
	return engine.toolSafetyClass(name)
}

func (r *Runner) handleRejectedToolCall(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
	decision ToolDecision,
) error {
	engine := &defaultEngine{runner: r}
	return engine.handleRejectedToolCall(ctx, sess, call, out, decision)
}

func classifyToolExecutionError(err error) (status, reasonCode string) {
	status = statusError
	reasonCode = string(tools.ToolErrorToolFailed)
	if err == nil {
		return status, reasonCode
	}

	var execErr *tools.ToolExecError
	if errors.As(err, &execErr) && execErr != nil {
		code := strings.TrimSpace(string(execErr.Code))
		if code != "" {
			reasonCode = code
		}
		if reasonCode == reasonCodePermissionDenied {
			status = statusDenied
		}
		return status, reasonCode
	}

	var runtimeErr runtimeTaskResultError
	if errors.As(err, &runtimeErr) {
		if code := strings.TrimSpace(runtimeErr.errorCode); code != "" {
			reasonCode = code
		} else {
			reasonCode = "runtime_task_error"
		}
	}
	return status, reasonCode
}

func toSandboxExecRules(rules []configpkg.ExecAllowRule) []sandboxpkg.ExecRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]sandboxpkg.ExecRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, sandboxpkg.ExecRule{
			Command:     rule.Command,
			ArgsPattern: append([]string(nil), rule.ArgsPattern...),
		})
	}
	return out
}

func buildSandboxRoots(workspace string, writableRoots []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(writableRoots)+1)
	appendRoot := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		if _, exists := seen[root]; exists {
			return
		}
		seen[root] = struct{}{}
		out = append(out, root)
	}
	appendRoot(workspace)
	for _, root := range writableRoots {
		appendRoot(root)
	}
	return out
}

func toSandboxNetworkRules(rules []configpkg.NetworkAllowRule) []sandboxpkg.NetworkRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]sandboxpkg.NetworkRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, sandboxpkg.NetworkRule{
			Host:   rule.Host,
			Port:   rule.Port,
			Scheme: rule.Scheme,
		})
	}
	return out
}
