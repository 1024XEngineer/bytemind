package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
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

type sandboxAuditContext struct {
	Enabled         bool
	Mode            string
	Backend         string
	RequiredCapable bool
	CapabilityLevel string
	Fallback        bool
	Status          string
	FallbackReason  string
}

func (e *defaultEngine) executeToolCall(
	ctx context.Context,
	sess *session.Session,
	runMode planpkg.AgentMode,
	call llm.ToolCall,
	out io.Writer,
	allowedTools map[string]struct{},
	deniedTools map[string]struct{},
	approval tools.ApprovalHandler,
	sandboxAudit sandboxAuditContext,
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
	sandboxAudit = normalizeSandboxAuditContext(sandboxAudit)

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
	permissionMetadata := toolAuditMetadata(call.Function.Name, map[string]string{
		"reason": decision.Reason,
	})
	appendSandboxAuditContext(permissionMetadata, sandboxAudit)
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID:  sessionID,
		TraceID:    traceID,
		Actor:      "agent",
		Action:     "permission_decision",
		Decision:   decision.Decision,
		ReasonCode: decision.ReasonCode,
		RiskLevel:  decision.RiskLevel,
		Metadata:   permissionMetadata,
	})

	if decision.Decision == corepkg.DecisionDeny {
		return e.handleRejectedToolCall(ctx, sess, call, out, decision, sandboxAudit)
	}

	runner.emit(Event{
		Type:          EventToolCallStarted,
		SessionID:     sessionID,
		ToolName:      call.Function.Name,
		ToolArguments: call.Function.Arguments,
	})
	sandboxLeaseID := fmt.Sprintf("session-%s", sess.ID)
	sandboxRunID := fmt.Sprintf("trace-%s", traceID)
	startMetadata := toolAuditMetadata(call.Function.Name, map[string]string{
		"sandbox_lease_id": sandboxLeaseID,
		"sandbox_run_id":   sandboxRunID,
	})
	appendSandboxAuditContext(startMetadata, sandboxAudit)
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: sessionID,
		TraceID:   traceID,
		Actor:     "agent",
		Action:    "tool_execute_start",
		Metadata:  startMetadata,
	})
	if out != nil {
		_, _ = io.WriteString(out, ansiBold+ansiCyan+"tool>"+ansiReset+" "+call.Function.Name+"\n")
	}

	execStartedAt := time.Now()
	runtimeMetadata := map[string]string{
		"tool_name": call.Function.Name,
	}
	appendSandboxAuditContext(runtimeMetadata, sandboxAudit)
	execution, runtimeErr := runner.runtime.RunSync(ctx, RuntimeTaskRequest{
		SessionID: sessionID,
		TraceID:   traceID,
		Name:      call.Function.Name,
		Kind:      "tool",
		Metadata:  runtimeMetadata,
		Execute: func(execCtx context.Context) ([]byte, error) {
			sandboxRoots := buildSandboxRoots(runner.workspace, runner.config.WritableRoots)
			output, err := runner.executor.ExecuteForMode(execCtx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
				Workspace:         runner.workspace,
				WritableRoots:     runner.config.WritableRoots,
				ApprovalPolicy:    runner.config.ApprovalPolicy,
				ApprovalMode:      runner.config.ApprovalMode,
				AwayPolicy:        runner.config.AwayPolicy,
				SandboxEnabled:    runner.config.SandboxEnabled,
				SystemSandboxMode: runner.config.SystemSandboxMode,
				LeaseID:           sandboxLeaseID,
				RunID:             sandboxRunID,
				FSRead:            append([]string(nil), sandboxRoots...),
				FSWrite:           append([]string(nil), sandboxRoots...),
				ExecAllowlist:     toSandboxExecRules(runner.config.ExecAllowlist),
				NetworkAllowlist:  toSandboxNetworkRules(runner.config.NetworkAllowlist),
				Approval:          approval,
				Session:           sess,
				TaskManager:       runner.taskManager,
				Extensions:        runner.extensions,
				Mode:              runMode,
				Stdin:             runner.stdin,
				Stdout:            runner.stdout,
				AllowedTools:      allowedTools,
				DeniedTools:       deniedTools,
			})
			return []byte(output), err
		},
		OnTaskStateChanged: func(task runtimepkg.Task) {
			runner.appendTaskStateAudit(
				ctx,
				sessionID,
				traceID,
				call.Function.Name,
				sandboxAudit,
				task,
			)
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
	metadata := toolAuditMetadata(call.Function.Name, map[string]string{
		"error":            errText,
		"sandbox_lease_id": sandboxLeaseID,
		"sandbox_run_id":   sandboxRunID,
	})
	appendSandboxAuditContext(metadata, sandboxAudit)
	if execution.Result.ErrorCode != "" {
		metadata["error_code"] = execution.Result.ErrorCode
	}
	appendSystemSandboxAuditMetadata(metadata, result)
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
	sandboxAudit sandboxAuditContext,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner
	sandboxAudit = normalizeSandboxAuditContext(sandboxAudit)

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

	deniedMetadata := toolAuditMetadata(call.Function.Name, map[string]string{
		"error":    errorText,
		"decision": string(decision.Decision),
	})
	appendSandboxAuditContext(deniedMetadata, sandboxAudit)
	runner.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
		Actor:     "agent",
		Action:    "tool_execute_result",
		Result:    "denied",
		Metadata:  deniedMetadata,
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
	sandboxAudit sandboxAuditContext,
) error {
	engine := &defaultEngine{runner: r}
	return engine.executeToolCall(ctx, sess, runMode, call, out, allowedTools, deniedTools, approval, sandboxAudit)
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
	sandboxAudit sandboxAuditContext,
) error {
	engine := &defaultEngine{runner: r}
	return engine.handleRejectedToolCall(ctx, sess, call, out, decision, sandboxAudit)
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

func appendSystemSandboxAuditMetadata(metadata map[string]string, result string) {
	if len(metadata) == 0 || strings.TrimSpace(result) == "" {
		return
	}
	var payload struct {
		SystemSandbox *struct {
			Mode            string `json:"mode"`
			Backend         string `json:"backend"`
			Status          string `json:"status"`
			RequiredCapable bool   `json:"required_capable"`
			CapabilityLevel string `json:"capability_level"`
			Fallback        bool   `json:"fallback"`
			FallbackReason  string `json:"fallback_reason"`
		} `json:"system_sandbox"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil || payload.SystemSandbox == nil {
		return
	}
	systemSandbox := payload.SystemSandbox
	if mode := strings.TrimSpace(systemSandbox.Mode); mode != "" {
		metadata["sandbox_mode"] = mode
	}
	if backend := strings.TrimSpace(systemSandbox.Backend); backend != "" {
		metadata["sandbox_backend"] = backend
	}
	if status := strings.TrimSpace(systemSandbox.Status); status != "" {
		metadata["sandbox_status"] = status
	}
	metadata["sandbox_required_capable"] = strconv.FormatBool(systemSandbox.RequiredCapable)
	if capability := strings.TrimSpace(systemSandbox.CapabilityLevel); capability != "" {
		metadata["sandbox_capability_level"] = capability
	}
	metadata["sandbox_fallback"] = strconv.FormatBool(systemSandbox.Fallback)
	if reason := strings.TrimSpace(systemSandbox.FallbackReason); reason != "" {
		metadata["sandbox_fallback_reason"] = reason
	}
}

func sandboxAuditFromSetup(setup runPromptSetup, sandboxEnabled bool, configuredMode string) sandboxAuditContext {
	context := sandboxAuditContext{
		Enabled:         sandboxEnabled,
		Mode:            strings.TrimSpace(configuredMode),
		Backend:         strings.TrimSpace(setup.SystemSandboxBackend),
		RequiredCapable: setup.SystemSandboxRequiredCapable,
		CapabilityLevel: strings.TrimSpace(setup.SystemSandboxCapabilityLevel),
		Fallback:        setup.SystemSandboxFallback,
		FallbackReason:  strings.TrimSpace(setup.SystemSandboxStatus),
	}
	if context.Fallback {
		context.Status = "fallback"
	} else if !context.Enabled || strings.EqualFold(context.Mode, "off") {
		context.Status = "off"
		context.FallbackReason = ""
	} else if strings.EqualFold(context.Backend, "none") {
		context.Status = "inactive"
		context.FallbackReason = ""
	} else {
		context.Status = "active"
		context.FallbackReason = ""
	}
	return normalizeSandboxAuditContext(context)
}

func normalizeSandboxAuditContext(context sandboxAuditContext) sandboxAuditContext {
	context.Mode = strings.TrimSpace(context.Mode)
	if context.Mode == "" {
		context.Mode = "off"
	}
	context.Backend = strings.TrimSpace(context.Backend)
	if context.Backend == "" {
		context.Backend = "none"
	}
	context.CapabilityLevel = strings.TrimSpace(context.CapabilityLevel)
	if context.CapabilityLevel == "" {
		context.CapabilityLevel = "none"
	}
	context.Status = strings.TrimSpace(context.Status)
	context.FallbackReason = strings.TrimSpace(context.FallbackReason)
	return context
}

func appendSandboxAuditContext(metadata map[string]string, context sandboxAuditContext) {
	if len(metadata) == 0 {
		return
	}
	context = normalizeSandboxAuditContext(context)
	metadata["sandbox_enabled"] = strconv.FormatBool(context.Enabled)
	metadata["sandbox_mode"] = context.Mode
	metadata["sandbox_backend"] = context.Backend
	metadata["sandbox_required_capable"] = strconv.FormatBool(context.RequiredCapable)
	metadata["sandbox_capability_level"] = context.CapabilityLevel
	metadata["sandbox_fallback"] = strconv.FormatBool(context.Fallback)
	if context.Status != "" {
		metadata["sandbox_status"] = context.Status
	}
	if context.FallbackReason != "" {
		metadata["sandbox_fallback_reason"] = context.FallbackReason
	}
}
