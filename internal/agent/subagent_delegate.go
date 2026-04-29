package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	corepkg "bytemind/internal/core"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
)

const (
	subAgentErrorCodeNotImplemented        = "subagent_not_implemented"
	subAgentErrorCodeRuntimeUnavailable    = "subagent_runtime_unavailable"
	subAgentErrorCodeBackgroundUnsupported = "subagent_background_not_supported"
	subAgentErrorCodeInvalidResult         = "subagent_invalid_result"

	subAgentResultStatusCompleted = "completed"
	subAgentResultStatusFailed    = "failed"
)

var subAgentInvocationCounter atomic.Uint64

func (r *Runner) delegateSubAgent(
	ctx context.Context,
	request tools.DelegateSubAgentRequest,
	execCtx *tools.ExecutionContext,
) (tools.DelegateSubAgentResult, error) {
	result := tools.DelegateSubAgentResult{
		OK:           false,
		Status:       subAgentResultStatusFailed,
		InvocationID: newSubAgentInvocationID(),
		Agent:        request.Agent,
		Findings:     []tools.DelegateSubAgentFinding{},
		References:   []tools.DelegateSubAgentReference{},
	}

	if r == nil || r.subAgentManager == nil {
		result.Error = &tools.DelegateSubAgentError{
			Code:      subagentspkg.ErrorCodeSubAgentUnavailable,
			Message:   "subagent manager is unavailable",
			Retryable: true,
		}
		return result, nil
	}

	runMode := planpkg.ModeBuild
	if execCtx != nil {
		runMode = planpkg.NormalizeMode(string(execCtx.Mode))
	}
	parentVisible := []string(nil)
	if r.registry != nil {
		parentVisible = toolNames(r.registry.DefinitionsForMode(runMode))
	}

	gateway := subagentspkg.NewGateway(r.subAgentManager)
	preflight, err := gateway.Preflight(subagentspkg.PreflightRequest{
		Agent:              request.Agent,
		Task:               request.Task,
		Mode:               runMode,
		ParentVisible:      parentVisible,
		ParentAllowed:      cloneToolSet(execCtxGetAllowed(execCtx)),
		ParentDenied:       cloneToolSet(execCtxGetDenied(execCtx)),
		RequestedTimeout:   request.Timeout,
		RequestedOutput:    request.Output,
		RequestedIsolation: request.Isolation,
	})
	if err != nil {
		if gatewayErr, ok := err.(*subagentspkg.GatewayError); ok {
			result.Error = &tools.DelegateSubAgentError{
				Code:      gatewayErr.Code,
				Message:   gatewayErr.Message,
				Retryable: gatewayErr.Retryable,
			}
			return result, nil
		}
		result.Error = &tools.DelegateSubAgentError{
			Code:      subagentspkg.ErrorCodeSubAgentUnavailable,
			Message:   err.Error(),
			Retryable: true,
		}
		return result, nil
	}
	if canonical := strings.TrimSpace(preflight.Definition.Name); canonical != "" {
		result.Agent = canonical
	}

	if request.RunInBackground {
		result.Error = &tools.DelegateSubAgentError{
			Code:      subAgentErrorCodeBackgroundUnsupported,
			Message:   "run_in_background is not wired yet for delegate_subagent",
			Retryable: false,
		}
		return result, nil
	}

	if r.runtime == nil {
		result.Error = &tools.DelegateSubAgentError{
			Code:      subAgentErrorCodeRuntimeUnavailable,
			Message:   "runtime gateway is unavailable for subagent execution",
			Retryable: true,
		}
		return result, nil
	}

	metadata := map[string]string{
		"invocation_id":          result.InvocationID,
		"agent":                  result.Agent,
		"mode":                   string(runMode),
		"isolation":              preflight.Isolation,
		"effective_tool_count":   strconv.Itoa(len(preflight.EffectiveTools)),
		"effective_toolset_hash": effectiveToolsetHash(preflight.EffectiveTools),
	}
	if preflight.RequestedTimeout != "" {
		metadata["requested_timeout"] = preflight.RequestedTimeout
	}
	if preflight.RequestedOutput != "" {
		metadata["requested_output"] = preflight.RequestedOutput
	}

	execution, runErr := r.runtime.RunSync(ctx, RuntimeTaskRequest{
		SessionID: sessionIDFromExecutionContext(execCtx),
		Name:      "delegate_subagent/" + preflightResultName(result.Agent),
		Kind:      "subagent",
		Metadata:  metadata,
		Execute: func(taskCtx context.Context) ([]byte, error) {
			_ = taskCtx
			return nil, &subAgentExecutionError{
				code:      subAgentErrorCodeNotImplemented,
				message:   "subagent execution pipeline is not wired yet",
				retryable: true,
			}
		},
	})
	if runErr != nil {
		if execution.TaskID != "" {
			result.TaskID = string(execution.TaskID)
		}
		result.Error = mapDelegateSubAgentError(runErr, subAgentErrorCodeRuntimeUnavailable)
		return result, nil
	}
	if execution.TaskID != "" {
		result.TaskID = string(execution.TaskID)
	}
	if execution.ExecutionError != nil {
		result.Error = mapDelegateSubAgentError(execution.ExecutionError, subAgentErrorCodeNotImplemented)
		return result, nil
	}
	if execution.Result.Status != corepkg.TaskCompleted {
		errorCode := strings.TrimSpace(execution.Result.ErrorCode)
		if errorCode == "" {
			errorCode = subAgentErrorCodeRuntimeUnavailable
		}
		result.Error = &tools.DelegateSubAgentError{
			Code:      errorCode,
			Message:   fmt.Sprintf("subagent task ended with status %s", execution.Result.Status),
			Retryable: execution.Result.Status != corepkg.TaskKilled,
		}
		return result, nil
	}
	if len(strings.TrimSpace(string(execution.Result.Output))) > 0 {
		normalized, normalizeErr := normalizeDelegateSubAgentResult(
			execution.Result.Output,
			result.InvocationID,
			request.Agent,
			result.TaskID,
		)
		if normalizeErr != nil {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeInvalidResult,
				Message:   fmt.Sprintf("subagent returned invalid structured result: %v", normalizeErr),
				Retryable: true,
			}
			return result, nil
		}
		return normalized, nil
	}

	result.Error = &tools.DelegateSubAgentError{
		Code:      subAgentErrorCodeNotImplemented,
		Message:   "subagent execution returned no structured result",
		Retryable: true,
	}
	return result, nil
}

func preflightResultName(agent string) string {
	name := strings.TrimSpace(agent)
	if name == "" {
		return "unknown"
	}
	return name
}

func sessionIDFromExecutionContext(execCtx *tools.ExecutionContext) corepkg.SessionID {
	if execCtx == nil || execCtx.Session == nil {
		return ""
	}
	return corepkg.SessionID(execCtx.Session.ID)
}

type subAgentExecutionError struct {
	code      string
	message   string
	retryable bool
}

func (e *subAgentExecutionError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.message)
}

func mapDelegateSubAgentError(err error, fallbackCode string) *tools.DelegateSubAgentError {
	if err == nil {
		return nil
	}
	var executionErr *subAgentExecutionError
	if errors.As(err, &executionErr) && executionErr != nil {
		code := strings.TrimSpace(executionErr.code)
		if code == "" {
			code = fallbackCode
		}
		return &tools.DelegateSubAgentError{
			Code:      code,
			Message:   strings.TrimSpace(executionErr.message),
			Retryable: executionErr.retryable,
		}
	}
	var runtimeErr interface{ Code() string }
	if errors.As(err, &runtimeErr) {
		code := strings.TrimSpace(runtimeErr.Code())
		if code == "" {
			code = fallbackCode
		}
		retryable := true
		var semanticRetryable interface{ Retryable() bool }
		if errors.As(err, &semanticRetryable) {
			retryable = semanticRetryable.Retryable()
		} else {
			retryable = code != runtimepkg.ErrorCodeTaskCancelled
		}
		return &tools.DelegateSubAgentError{
			Code:      code,
			Message:   strings.TrimSpace(err.Error()),
			Retryable: retryable,
		}
	}
	return &tools.DelegateSubAgentError{
		Code:      fallbackCode,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: true,
	}
}

func normalizeDelegateSubAgentResult(
	payload []byte,
	fallbackInvocationID string,
	fallbackAgent string,
	fallbackTaskID string,
) (tools.DelegateSubAgentResult, error) {
	var result tools.DelegateSubAgentResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return tools.DelegateSubAgentResult{}, err
	}
	result.InvocationID = firstNonEmpty(result.InvocationID, fallbackInvocationID)
	result.Agent = firstNonEmpty(result.Agent, fallbackAgent)
	result.TaskID = firstNonEmpty(result.TaskID, fallbackTaskID)
	if result.Findings == nil {
		result.Findings = []tools.DelegateSubAgentFinding{}
	}
	if result.References == nil {
		result.References = []tools.DelegateSubAgentReference{}
	}
	if result.OK && result.Error != nil {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("ok result must not include error")
	}
	result.Status = strings.ToLower(strings.TrimSpace(result.Status))
	if result.Status == "" {
		if result.OK {
			result.Status = subAgentResultStatusCompleted
		} else {
			result.Status = subAgentResultStatusFailed
		}
	}
	if !result.OK {
		if result.Error == nil {
			return tools.DelegateSubAgentResult{}, fmt.Errorf("failed result must include error")
		}
		result.Error.Code = strings.TrimSpace(result.Error.Code)
		result.Error.Message = strings.TrimSpace(result.Error.Message)
		if result.Error.Code == "" || result.Error.Message == "" {
			return tools.DelegateSubAgentResult{}, fmt.Errorf("failed result must include non-empty error code/message")
		}
	}
	return result, nil
}

func firstNonEmpty(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func effectiveToolsetHash(toolNames []string) string {
	if len(toolNames) == 0 {
		return ""
	}
	canonical := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		canonical = append(canonical, trimmed)
	}
	if len(canonical) == 0 {
		return ""
	}
	sort.Strings(canonical)
	sum := sha256.Sum256([]byte(strings.Join(canonical, "\n")))
	return hex.EncodeToString(sum[:])
}

func execCtxGetAllowed(execCtx *tools.ExecutionContext) map[string]struct{} {
	if execCtx == nil {
		return nil
	}
	return execCtx.AllowedTools
}

func execCtxGetDenied(execCtx *tools.ExecutionContext) map[string]struct{} {
	if execCtx == nil {
		return nil
	}
	return execCtx.DeniedTools
}

func cloneToolSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]struct{}, len(source))
	for key := range source {
		cloned[key] = struct{}{}
	}
	return cloned
}

func newSubAgentInvocationID() string {
	sequence := subAgentInvocationCounter.Add(1)
	return fmt.Sprintf("subagent-%d-%d", time.Now().UTC().UnixNano(), sequence)
}
